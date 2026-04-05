package v1

import (
	"encoding/json"
	"fmt"
	"io"
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

const (
	weixinAPIBaseURL = "https://ilinkai.weixin.qq.com"
	weixinBotType    = "3"
	weixinChannelID  = "openclaw-weixin"
	weixinQRTTL      = 5 * time.Minute
)

// weixinLoginSession tracks an active QR login session
type weixinLoginSession struct {
	BotID     string
	Name      string // optional friendly name for the account
	QRCode    string // opaque token for polling status
	QRCodeURL string // image URL for the QR code
	CreatedAt time.Time
}

var (
	weixinSessionsMu sync.Mutex
	weixinSessions   = make(map[string]*weixinLoginSession) // botID -> session
)

// ilink API response types
type ilinkQRCodeResponse struct {
	QRCode         string `json:"qrcode"`
	QRCodeImgURL   string `json:"qrcode_img_content"`
}

type ilinkQRStatusResponse struct {
	Status      string `json:"status"` // wait, scaned, confirmed, expired
	BotToken    string `json:"bot_token,omitempty"`
	IlinkBotID  string `json:"ilink_bot_id,omitempty"`
	BaseURL     string `json:"baseurl,omitempty"`
	IlinkUserID string `json:"ilink_user_id,omitempty"`
}

// WechatLoginStart initiates a WeChat QR code login for a bot.
// POST /bot/api/v1/bots/:id/channels/wechat/login
//
// Response:
//
//	{
//	  "qrcode_url": "https://...",
//	  "message": "使用微信扫描二维码"
//	}
func WechatLoginStart(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}
	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running")
	}

	// Optional friendly name for the account
	var body struct {
		Name string `json:"name"`
	}
	c.Bind(&body)

	// Fetch QR code from ilink API
	qrResp, err := fetchWeixinQRCode()
	if err != nil {
		return util.InternalError(c, "failed to get QR code: "+err.Error())
	}

	// Store session for status polling
	weixinSessionsMu.Lock()
	weixinSessions[bot.ID] = &weixinLoginSession{
		BotID:     bot.ID,
		Name:      body.Name,
		QRCode:    qrResp.QRCode,
		QRCodeURL: qrResp.QRCodeImgURL,
		CreatedAt: time.Now(),
	}
	weixinSessionsMu.Unlock()

	return util.Success(c, map[string]any{
		"qrcode_url": qrResp.QRCodeImgURL,
		"message":    "使用微信扫描二维码，以完成连接。",
	})
}

// WechatLoginStatus polls the QR code scan status.
// GET /bot/api/v1/bots/:id/channels/wechat/login/status
//
// Response:
//
//	{
//	  "status": "wait|scaned|confirmed|expired",
//	  "connected": false,
//	  "message": "等待扫码..."
//	}
func WechatLoginStatus(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}

	// Get active session
	weixinSessionsMu.Lock()
	session := weixinSessions[bot.ID]
	weixinSessionsMu.Unlock()

	if session == nil {
		return util.BadRequest(c, "no active login session, call login first")
	}

	// Check if expired locally
	if time.Since(session.CreatedAt) > weixinQRTTL {
		weixinSessionsMu.Lock()
		delete(weixinSessions, bot.ID)
		weixinSessionsMu.Unlock()
		return util.Success(c, map[string]any{
			"status":    "expired",
			"connected": false,
			"message":   "二维码已过期，请重新获取。",
		})
	}

	// Poll ilink API for status
	statusResp, err := pollWeixinQRStatus(session.QRCode)
	if err != nil {
		return util.InternalError(c, "failed to poll status: "+err.Error())
	}

	switch statusResp.Status {
	case "confirmed":
		// Clean up session
		weixinSessionsMu.Lock()
		delete(weixinSessions, bot.ID)
		weixinSessionsMu.Unlock()

		// Write credentials to bot pod
		if statusResp.BotToken != "" && statusResp.IlinkBotID != "" {
			go writeWechatCredentials(bot, statusResp, session.Name)
		}

		return util.Success(c, map[string]any{
			"status":     "confirmed",
			"connected":  true,
			"account_id": statusResp.IlinkBotID,
			"message":    "微信连接成功！",
		})

	case "scaned":
		return util.Success(c, map[string]any{
			"status":    "scaned",
			"connected": false,
			"message":   "已扫码，请在微信上确认登录。",
		})

	case "expired":
		weixinSessionsMu.Lock()
		delete(weixinSessions, bot.ID)
		weixinSessionsMu.Unlock()
		return util.Success(c, map[string]any{
			"status":    "expired",
			"connected": false,
			"message":   "二维码已过期，请重新获取。",
		})

	default: // "wait"
		return util.Success(c, map[string]any{
			"status":    "wait",
			"connected": false,
			"message":   "等待扫码...",
		})
	}
}

// fetchWeixinQRCode calls the ilink API to get a new QR code
func fetchWeixinQRCode() (*ilinkQRCodeResponse, error) {
	apiURL := fmt.Sprintf("%s/ilink/bot/get_bot_qrcode?bot_type=%s", weixinAPIBaseURL, weixinBotType)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var qrResp ilinkQRCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&qrResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if qrResp.QRCode == "" {
		return nil, fmt.Errorf("empty QR code in response")
	}

	return &qrResp, nil
}

// pollWeixinQRStatus polls the ilink API for QR code scan status
func pollWeixinQRStatus(qrcode string) (*ilinkQRStatusResponse, error) {
	apiURL := fmt.Sprintf("%s/ilink/bot/get_qrcode_status?qrcode=%s",
		weixinAPIBaseURL, url.QueryEscape(qrcode))

	client := &http.Client{Timeout: 40 * time.Second} // long-poll timeout
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var statusResp ilinkQRStatusResponse
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &statusResp, nil
}

// normalizeAccountID converts raw account ID to filesystem-safe format
// e.g. "161325c4c670@im.bot" -> "161325c4c670-im-bot"
func normalizeAccountID(raw string) string {
	s := strings.ReplaceAll(raw, "@", "-")
	s = strings.ReplaceAll(s, ".", "-")
	return s
}

// writeWechatCredentials writes credential files to the bot's pod.
// The plugin reads credentials from these files, NOT from openclaw.json.
func writeWechatCredentials(bot *model.Bot, status *ilinkQRStatusResponse, name string) {
	ctx := context.Background()

	baseURL := status.BaseURL
	if baseURL == "" {
		baseURL = weixinAPIBaseURL
	}

	normalizedID := normalizeAccountID(status.IlinkBotID)

	podName, err := k8s.GetPodName(ctx, bot.ID)
	if err != nil {
		fmt.Printf("[Wechat] Failed to get pod: %v\n", err)
		return
	}
	ns := k8s.GetNamespace()

	// 1. Write credential file: ~/.openclaw/openclaw-weixin/accounts/{normalizedID}.json
	credData := map[string]string{
		"token":   status.BotToken,
		"savedAt": time.Now().UTC().Format(time.RFC3339),
		"baseUrl": baseURL,
		"userId":  status.IlinkUserID,
	}
	if name != "" {
		credData["name"] = name
	}
	credJSON, _ := json.Marshal(credData)

	// Create directory
	k8s.ExecInPod(ctx, ns, podName, "openclaw", []string{"mkdir", "-p", "/home/node/.openclaw/openclaw-weixin/accounts"})

	// Write credential file safely via stdin
	credPath := fmt.Sprintf("/home/node/.openclaw/openclaw-weixin/accounts/%s.json", normalizedID)
	if _, err := k8s.ExecInPodWithStdin(ctx, ns, podName, "openclaw",
		[]string{"tee", credPath}, string(credJSON)); err != nil {
		fmt.Printf("[Wechat] Failed to write credential file: %v\n", err)
		return
	}
	// Set permissions
	k8s.ExecInPod(ctx, ns, podName, "openclaw", []string{"chmod", "600", credPath})

	// 2. Update account index: ~/.openclaw/openclaw-weixin/accounts.json
	indexCmd := fmt.Sprintf(`node -e "
const fs = require('fs');
const f = '/home/node/.openclaw/openclaw-weixin/accounts.json';
let ids = [];
try { ids = JSON.parse(fs.readFileSync(f, 'utf8')); } catch {}
if (!ids.includes('%s')) { ids.push('%s'); fs.writeFileSync(f, JSON.stringify(ids, null, 2)); }
"`, normalizedID, normalizedID)
	if _, err := k8s.ExecInPod(ctx, ns, podName, "openclaw", []string{"sh", "-c", indexCmd}); err != nil {
		fmt.Printf("[Wechat] Failed to update account index: %v\n", err)
		return
	}

	// 3. Write account entry to openclaw.json and trigger channel reload
	triggerCmd := fmt.Sprintf(`node -e "
const fs = require('fs');
const f = '/home/node/.openclaw/openclaw.json';
const c = JSON.parse(fs.readFileSync(f, 'utf8'));
if (!c.channels) c.channels = {};
if (!c.channels['openclaw-weixin']) c.channels['openclaw-weixin'] = {};
c.channels['openclaw-weixin'].enabled = true;
if (!c.channels['openclaw-weixin'].accounts) c.channels['openclaw-weixin'].accounts = {};
c.channels['openclaw-weixin'].accounts['%s'] = {};
fs.writeFileSync(f, JSON.stringify(c, null, 2));
"`, normalizedID)
	if _, err := k8s.ExecInPod(ctx, ns, podName, "openclaw", []string{"sh", "-c", triggerCmd}); err != nil {
		fmt.Printf("[Wechat] Failed to trigger reload: %v\n", err)
	}

	fmt.Printf("[Wechat] Credentials written for bot %s, account=%s\n", bot.ID, normalizedID)
}

// WechatListAccounts lists all connected WeChat accounts for a bot.
// GET /bot/api/v1/bots/:id/channels/wechat/accounts
func WechatListAccounts(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}
	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running")
	}

	ctx := context.Background()
	podName, err := k8s.GetPodName(ctx, bot.ID)
	if err != nil {
		return util.InternalError(c, "failed to get pod: "+err.Error())
	}

	// Read account index and credential files
	output, err := k8s.ExecInPod(ctx, k8s.GetNamespace(), podName, "openclaw", []string{"sh", "-c", `node -e "
const fs = require('fs');
const indexFile = '/home/node/.openclaw/openclaw-weixin/accounts.json';
const accountsDir = '/home/node/.openclaw/openclaw-weixin/accounts';
let ids = [];
try { ids = JSON.parse(fs.readFileSync(indexFile, 'utf8')); } catch {}
const accounts = ids.map(id => {
  let data = {};
  try { data = JSON.parse(fs.readFileSync(accountsDir + '/' + id + '.json', 'utf8')); } catch {}
  return {
    account_id: id,
    name: data.name || id,
    base_url: data.baseUrl || '',
    user_id: data.userId || '',
    saved_at: data.savedAt || '',
    configured: Boolean(data.token)
  };
});
console.log(JSON.stringify(accounts));
"`})
	if err != nil {
		// No accounts yet
		return util.Success(c, map[string]any{
			"channel":  weixinChannelID,
			"accounts": []any{},
		})
	}

	var accounts []any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &accounts); err != nil {
		return util.Success(c, map[string]any{
			"channel":  weixinChannelID,
			"accounts": []any{},
		})
	}

	return util.Success(c, map[string]any{
		"channel":  weixinChannelID,
		"accounts": accounts,
	})
}

// WechatRemoveAccount removes a specific WeChat account from a bot.
// DELETE /bot/api/v1/bots/:id/channels/wechat/accounts/:account_id
func WechatRemoveAccount(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}
	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running")
	}

	accountID := c.Param("account_id")
	if accountID == "" {
		return util.BadRequest(c, "account_id is required")
	}

	ctx := context.Background()
	podName, err := k8s.GetPodName(ctx, bot.ID)
	if err != nil {
		return util.InternalError(c, "failed to get pod: "+err.Error())
	}
	ns := k8s.GetNamespace()

	// 1. Remove credential files (no shell interpolation)
	credFile := fmt.Sprintf("/home/node/.openclaw/openclaw-weixin/accounts/%s.json", accountID)
	syncFile := fmt.Sprintf("/home/node/.openclaw/openclaw-weixin/accounts/%s.sync.json", accountID)
	k8s.ExecInPod(ctx, ns, podName, "openclaw", []string{"rm", "-f", credFile, syncFile})

	// 2. Remove from account index
	indexCmd := fmt.Sprintf(`node -e "
const fs = require('fs');
const f = '/home/node/.openclaw/openclaw-weixin/accounts.json';
try {
  let ids = JSON.parse(fs.readFileSync(f, 'utf8'));
  ids = ids.filter(id => id !== '%s');
  fs.writeFileSync(f, JSON.stringify(ids, null, 2));
} catch {}
"`, accountID)
	k8s.ExecInPod(ctx, ns, podName, "openclaw", []string{"sh", "-c", indexCmd})

	// 3. Remove account from openclaw.json config; if no accounts left, remove entire channel
	configCmd := fmt.Sprintf(`node -e "
const fs = require('fs');
const f = '/home/node/.openclaw/openclaw.json';
try {
  const c = JSON.parse(fs.readFileSync(f, 'utf8'));
  if (c.channels && c.channels['openclaw-weixin']) {
    const ch = c.channels['openclaw-weixin'];
    if (ch.accounts) {
      delete ch.accounts['%s'];
      if (Object.keys(ch.accounts).length === 0) {
        delete c.channels['openclaw-weixin'];
      }
    } else {
      delete c.channels['openclaw-weixin'];
    }
    fs.writeFileSync(f, JSON.stringify(c, null, 2));
  }
} catch {}
"`, accountID)
	k8s.ExecInPod(ctx, ns, podName, "openclaw", []string{"sh", "-c", configCmd})

	return util.Success(c, map[string]any{
		"message":    "WeChat account removed",
		"channel":    weixinChannelID,
		"account_id": accountID,
	})
}
