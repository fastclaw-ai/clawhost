package proxy

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const sessionCookieName = "_claw_session"

// sessionCookieValue generates an HMAC-SHA256 signature for the session cookie.
// Using HMAC rather than storing the raw token prevents cookie leaks from exposing the access token.
func sessionCookieValue(botID, accessToken string) string {
	mac := hmac.New(sha256.New, []byte(accessToken))
	mac.Write([]byte(botID))
	return hex.EncodeToString(mac.Sum(nil))
}

// validateSession checks if the request carries a valid ?token= or session cookie.
// Returns the access token on success, or an empty string on failure.
func validateSession(c echo.Context, bot *model.Bot) (accessToken string, ok bool) {
	// 1. Check ?token= query parameter
	if token := c.QueryParam("token"); token != "" && token == bot.AccessToken {
		return bot.AccessToken, true
	}

	// 2. Check session cookie
	cookie, err := c.Cookie(sessionCookieName)
	if err == nil && cookie.Value == sessionCookieValue(bot.ID, bot.AccessToken) {
		return bot.AccessToken, true
	}

	return "", false
}

// setSessionCookie sets an HttpOnly session cookie so subsequent requests don't need ?token=.
func setSessionCookie(c echo.Context, bot *model.Bot) {
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionCookieValue(bot.ID, bot.AccessToken),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7, // 7 days
	})
}

// isUUID checks if a string is in UUID format
func isUUID(s string) bool {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	return uuidRegex.MatchString(strings.ToLower(s))
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// pairingErrorResponse represents the NOT_PAIRED error from OpenClaw gateway
type pairingErrorResponse struct {
	Code    string `json:"code"`
	Details struct {
		RequestID string `json:"requestId"`
	} `json:"details"`
	Message string `json:"message"`
}

// isNotPairedResponse checks if a JSON body is a NOT_PAIRED error
func isNotPairedResponse(body []byte) bool {
	var resp pairingErrorResponse
	return json.Unmarshal(body, &resp) == nil && resp.Code == "NOT_PAIRED"
}

// --- Poller-based auto-approval ---
// When a request with a valid ?token= arrives (initial page load),
// we start a short-lived poller that repeatedly checks for pending devices.
// This handles the race condition where the WebUI JS creates pending device
// requests after the initial page has loaded.
// No token caching to avoid multi-tenancy issues — only the authenticated
// request triggers approval, and only for a limited window.

var (
	activePollersMu sync.Mutex
	activePollers   = make(map[string]bool) // botID -> polling in progress
)

// autoApprovePoller polls for pending devices and approves them over a short period.
func autoApprovePoller(botID, accessToken string) {
	// Prevent duplicate pollers for the same bot
	activePollersMu.Lock()
	if activePollers[botID] {
		activePollersMu.Unlock()
		return
	}
	activePollers[botID] = true
	activePollersMu.Unlock()

	defer func() {
		activePollersMu.Lock()
		delete(activePollers, botID)
		activePollersMu.Unlock()
	}()

	ctx := context.Background()
	// Poll every 2 seconds for ~16 seconds to catch newly pending devices
	for i := 0; i < 8; i++ {
		time.Sleep(2 * time.Second)
		if err := k8s.AutoApproveAllPending(ctx, botID, accessToken); err != nil {
			fmt.Printf("[AutoApprove] Poller error for bot %s: %v\n", botID, err)
		}
	}
}

// ProxyToBot proxies requests to the OpenClaw bot
// Path format: /proxy/{bot_id_or_slug}/*
func ProxyToBot(c echo.Context) error {
	botIdentifier := c.Param("bot_id")
	if botIdentifier == "" {
		return util.BadRequest(c, "bot_id or slug is required")
	}

	// Get bot info - determine if it's an ID (UUID format) or slug (short string)
	var bot *model.Bot
	var err error
	if isUUID(botIdentifier) {
		bot, err = model.GetBotByID(botIdentifier)
	} else {
		bot, err = model.GetBotBySlug(botIdentifier)
	}
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return util.NotFound(c, "bot not found")
		}
		return util.InternalError(c, "failed to get bot")
	}

	// When ChatClaw is enabled, require valid token or session cookie for all requests.
	// When using OpenClaw's built-in WebUI, security is handled by device pairing.
	chatclawMode := k8s.ChatClawEnabled()

	accessToken := ""
	if chatclawMode {
		token, ok := validateSession(c, bot)
		if !ok {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "access token required, use ?token=<access_token>",
			})
		}
		accessToken = token
		// Set session cookie so subsequent requests (JS/CSS/API) don't need ?token=
		setSessionCookie(c, bot)
	} else {
		// Legacy OpenClaw WebUI: auto-approval via polling
		if token := c.QueryParam("token"); token != "" && token == bot.AccessToken {
			accessToken = bot.AccessToken
			go autoApprovePoller(bot.ID, accessToken)
		}
	}

	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running")
	}

	// Get target URL from K8s service (uses ClusterIP in local dev mode, DNS in production)
	// When ChatClaw is enabled, routes to ChatClaw port; otherwise to OpenClaw gateway
	targetHost, err := k8s.GetWebUIEndpoint(context.Background(), bot.ID)
	if err != nil {
		return util.InternalError(c, "failed to get service endpoint")
	}
	if targetHost == "" {
		return util.NotFound(c, "bot service not found")
	}

	// Get the remaining path after /proxy/{bot_id}
	remainingPath := c.Param("*")
	if remainingPath == "" {
		remainingPath = "/"
	} else if !strings.HasPrefix(remainingPath, "/") {
		remainingPath = "/" + remainingPath
	}

	// Check if this is a WebSocket upgrade request
	if isWebSocketRequest(c.Request()) {
		return proxyWebSocket(c, targetHost, remainingPath, bot.ID, accessToken)
	}

	// Regular HTTP proxy
	targetURL := &url.URL{
		Scheme: "http",
		Host:   targetHost,
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetHost
		req.URL.Path = remainingPath
		req.URL.RawQuery = c.QueryString()

		// Forward real client IP
		clientIP := c.RealIP()
		req.Header.Set("X-Real-IP", clientIP)
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			req.Header.Set("X-Forwarded-For", xff+", "+clientIP)
		} else {
			req.Header.Set("X-Forwarded-For", clientIP)
		}
	}

	// Auto-approve NOT_PAIRED HTTP responses so subsequent client retries succeed
	if accessToken != "" {
		botID := bot.ID
		token := accessToken
		proxy.ModifyResponse = func(resp *http.Response) error {
			if resp.StatusCode < 400 {
				return nil
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil
			}
			resp.Body = io.NopCloser(bytes.NewReader(body))

			if isNotPairedResponse(body) {
				fmt.Printf("[Proxy] NOT_PAIRED detected in HTTP response for bot %s, auto-approving...\n", botID)
				go func() {
					ctx := context.Background()
					if err := k8s.AutoApproveAllPending(ctx, botID, token); err != nil {
						fmt.Printf("[Proxy] Auto-approve (HTTP) failed for bot %s: %v\n", botID, err)
					}
				}()
			}
			return nil
		}
	}

	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func isWebSocketRequest(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}

// buildWSRequestHeaders builds the headers for the backend WebSocket connection
func buildWSRequestHeaders(c echo.Context, targetHost string) http.Header {
	requestHeader := http.Header{}
	// Set Origin to the target host to pass OpenClaw's origin check
	// OpenClaw doesn't support wildcard "*" in allowedOrigins
	requestHeader.Set("Origin", fmt.Sprintf("http://%s", targetHost))
	if protocol := c.Request().Header.Get("Sec-WebSocket-Protocol"); protocol != "" {
		requestHeader.Set("Sec-WebSocket-Protocol", protocol)
	}
	// Forward Authorization header for password auth
	if auth := c.Request().Header.Get("Authorization"); auth != "" {
		requestHeader.Set("Authorization", auth)
	}
	// Forward Cookie header (OpenClaw may use cookie for session)
	if cookie := c.Request().Header.Get("Cookie"); cookie != "" {
		requestHeader.Set("Cookie", cookie)
	}
	// Forward real client IP
	clientIP := c.RealIP()
	requestHeader.Set("X-Real-IP", clientIP)
	if xff := c.Request().Header.Get("X-Forwarded-For"); xff != "" {
		requestHeader.Set("X-Forwarded-For", xff+", "+clientIP)
	} else {
		requestHeader.Set("X-Forwarded-For", clientIP)
	}
	return requestHeader
}

func proxyWebSocket(c echo.Context, targetHost, path, botID, accessToken string) error {
	// Upgrade client connection
	clientConn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer clientConn.Close()

	// Connect to backend WebSocket
	backendURL := url.URL{
		Scheme:   "ws",
		Host:     targetHost,
		Path:     path,
		RawQuery: c.QueryString(),
	}

	requestHeader := buildWSRequestHeaders(c, targetHost)

	// Dial backend with auto-approval retry for NOT_PAIRED errors
	backendConn, resp, err := websocket.DefaultDialer.Dial(backendURL.String(), requestHeader)

	// Handle NOT_PAIRED during WebSocket handshake (upgrade rejected with HTTP error)
	if err != nil && accessToken != "" && resp != nil && resp.Body != nil {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr == nil && isNotPairedResponse(body) {
			fmt.Printf("[Proxy] NOT_PAIRED detected during WS handshake for bot %s, auto-approving...\n", botID)

			// Approve all pending devices (synchronous - wait for completion before retry)
			ctx := context.Background()
			if approveErr := k8s.AutoApproveAllPending(ctx, botID, accessToken); approveErr != nil {
				fmt.Printf("[Proxy] Auto-approve (WS) failed for bot %s: %v\n", botID, approveErr)
			}

			// Retry WebSocket connection after approval
			backendConn, _, err = websocket.DefaultDialer.Dial(backendURL.String(), requestHeader)
			if err == nil {
				fmt.Printf("[Proxy] WS retry succeeded for bot %s after auto-approval\n", botID)
			}
		}
	}

	if err != nil {
		c.Logger().Errorf("WebSocket dial error: %v, url: %s", err, backendURL.String())
		return err
	}
	defer backendConn.Close()

	// Bidirectional message forwarding
	errCh := make(chan error, 2)

	// Client -> Backend
	go func() {
		for {
			msgType, msg, err := clientConn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			if err := backendConn.WriteMessage(msgType, msg); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Backend -> Client
	// If the WebSocket upgrade succeeded but NOT_PAIRED comes as a message,
	// detect it on the first message and trigger auto-approval in the background
	go func() {
		firstMessage := true
		for {
			msgType, msg, err := backendConn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}

			// Check first message for NOT_PAIRED (handles the case where
			// WebSocket upgrade succeeds but pairing is checked at message level)
			if firstMessage && accessToken != "" {
				firstMessage = false
				if isNotPairedResponse(msg) {
					fmt.Printf("[Proxy] NOT_PAIRED detected in WS message for bot %s, auto-approving...\n", botID)
					go func() {
						ctx := context.Background()
						if err := k8s.AutoApproveAllPending(ctx, botID, accessToken); err != nil {
							fmt.Printf("[Proxy] Auto-approve (WS msg) failed for bot %s: %v\n", botID, err)
						}
					}()
				}
			}

			if err := clientConn.WriteMessage(msgType, msg); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Wait for either direction to close
	<-errCh
	return nil
}
