package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ChannelConfig represents a channel configuration
type ChannelConfig struct {
	Token         string                 `json:"token,omitempty"`
	BotToken      string                 `json:"botToken,omitempty"`      // Slack
	AppToken      string                 `json:"appToken,omitempty"`      // Slack
	AppID         string                 `json:"appId,omitempty"`         // Feishu, Teams
	AppSecret     string                 `json:"appSecret,omitempty"`     // Feishu
	AppPassword   string                 `json:"appPassword,omitempty"`   // Teams
	ChannelSecret string                 `json:"channelSecret,omitempty"` // LINE
	Extra         map[string]interface{} `json:"extra,omitempty"`         // Additional config
}

// AddChannelToBot adds an IM channel account to a bot's OpenClaw instance
// This writes directly to the config file, openclaw will hot-reload
// Supports multi-account: channels.telegram.accounts.{accountName}
func AddChannelToBot(ctx context.Context, botID, accessToken, channel, account string, channelConfig map[string]interface{}) error {
	namespace := GetNamespace()

	podName, err := WaitForPodReady(ctx, botID, 30)
	if err != nil {
		return fmt.Errorf("failed to get pod: %w", err)
	}

	// Read existing config
	existingConfig, err := readOpenClawConfig(ctx, namespace, podName)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Separate channel-level fields from account-level fields
	// Channel-level: dmPolicy, groupPolicy, enabled, allowFrom (same level as "accounts")
	// Account-level: botToken, appId, appSecret, etc. (inside accounts.{name})
	channelLevelKeys := map[string]bool{
		"dmPolicy": true, "groupPolicy": true,
		"enabled": true, "allowFrom": true,
	}
	channelLevelConfig := make(map[string]interface{})
	accountConfig := make(map[string]interface{})
	for k, v := range channelConfig {
		if channelLevelKeys[k] {
			channelLevelConfig[k] = v
		} else {
			accountConfig[k] = v
		}
	}

	// Set channel-level defaults if not provided by upstream
	if _, ok := channelLevelConfig["dmPolicy"]; !ok {
		channelLevelConfig["dmPolicy"] = "open"
	}
	if _, ok := channelLevelConfig["enabled"]; !ok {
		channelLevelConfig["enabled"] = true
	}

	// Add/update channel account in config using multi-account structure
	// Structure: channels.{channel}.{enabled, dmPolicy, ...}.accounts.{account}
	if existingConfig["channels"] == nil {
		existingConfig["channels"] = make(map[string]interface{})
	}
	channels := existingConfig["channels"].(map[string]interface{})

	// Get or create channel config
	if channels[channel] == nil {
		channels[channel] = make(map[string]interface{})
	}
	chCfg, ok := channels[channel].(map[string]interface{})
	if !ok {
		chCfg = make(map[string]interface{})
		channels[channel] = chCfg
	}

	// Apply channel-level config (upstream values override existing)
	for k, v := range channelLevelConfig {
		chCfg[k] = v
	}

	// Get or create accounts map
	if chCfg["accounts"] == nil {
		chCfg["accounts"] = make(map[string]interface{})
	}
	accounts, ok := chCfg["accounts"].(map[string]interface{})
	if !ok {
		accounts = make(map[string]interface{})
		chCfg["accounts"] = accounts
	}

	// Add/update the account
	accounts[account] = accountConfig

	// Write updated config
	if err := writeOpenClawConfig(ctx, namespace, podName, existingConfig); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Sync config to database
	if err := SyncConfigToDatabase(ctx, botID); err != nil {
		// Log but don't fail the operation
		fmt.Printf("Warning: failed to sync config to database: %v\n", err)
	}

	return nil
}

// ChannelAccountInfo represents a channel account in the list
type ChannelAccountInfo struct {
	Channel  string                 `json:"channel"`
	Account  string                 `json:"account"`
	Name     string                 `json:"name,omitempty"`
	Status   string                 `json:"status"`
	Config   map[string]interface{} `json:"config,omitempty"`
}

// ListBotChannels lists all configured channel accounts for a bot
func ListBotChannels(ctx context.Context, botID, accessToken string) ([]ChannelAccountInfo, error) {
	namespace := GetNamespace()

	podName, err := WaitForPodReady(ctx, botID, 30)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	// Read config file
	config, err := readOpenClawConfig(ctx, namespace, podName)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Extract channels and accounts
	var result []ChannelAccountInfo
	if channels, ok := config["channels"].(map[string]interface{}); ok {
		for channelName, channelData := range channels {
			channelConfig, ok := channelData.(map[string]interface{})
			if !ok {
				continue
			}

			// Check for multi-account structure
			if accounts, ok := channelConfig["accounts"].(map[string]interface{}); ok {
				for accountName, accountData := range accounts {
					accountConfig, _ := accountData.(map[string]interface{})
					result = append(result, ChannelAccountInfo{
						Channel: channelName,
						Account: accountName,
						Status:  "configured",
						Config:  accountConfig,
					})
				}
			} else {
				// Legacy single-account structure
				result = append(result, ChannelAccountInfo{
					Channel: channelName,
					Account: "default",
					Status:  "configured",
					Config:  channelConfig,
				})
			}
		}
	}

	// For openclaw-weixin accounts, enrich with name from credential files
	enrichWeixinAccountNames(ctx, namespace, podName, result)

	return result, nil
}

// enrichWeixinAccountNames reads name from WeChat credential files and sets the Name field
func enrichWeixinAccountNames(ctx context.Context, namespace, podName string, accounts []ChannelAccountInfo) {
	for i := range accounts {
		if accounts[i].Channel != "openclaw-weixin" {
			continue
		}
		// Read credential file for this account
		credPath := fmt.Sprintf("/home/node/.openclaw/openclaw-weixin/accounts/%s.json", accounts[i].Account)
		output, err := ExecInPod(ctx, namespace, podName, "openclaw",
			[]string{"cat", credPath})
		if err != nil {
			accounts[i].Name = accounts[i].Account
			continue
		}
		var cred struct {
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(output), &cred) == nil && cred.Name != "" {
			accounts[i].Name = cred.Name
		} else {
			accounts[i].Name = accounts[i].Account
		}
	}
}

// RemoveChannelFromBot removes an IM channel account from a bot
// If account is empty, removes the entire channel; otherwise removes specific account
func RemoveChannelFromBot(ctx context.Context, botID, accessToken, channel, account string) error {
	namespace := GetNamespace()

	podName, err := WaitForPodReady(ctx, botID, 30)
	if err != nil {
		return fmt.Errorf("failed to get pod: %w", err)
	}

	// Read existing config
	existingConfig, err := readOpenClawConfig(ctx, namespace, podName)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Remove channel or account from config
	if channels, ok := existingConfig["channels"].(map[string]interface{}); ok {
		if account == "" {
			// Remove entire channel
			delete(channels, channel)
		} else {
			// Remove specific account
			if channelConfig, ok := channels[channel].(map[string]interface{}); ok {
				if accounts, ok := channelConfig["accounts"].(map[string]interface{}); ok {
					delete(accounts, account)
					// If no accounts left, remove the channel
					if len(accounts) == 0 {
						delete(channels, channel)
					}
				}
			}
		}
	}

	// Write updated config
	if err := writeOpenClawConfig(ctx, namespace, podName, existingConfig); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Sync config to database
	if err := SyncConfigToDatabase(ctx, botID); err != nil {
		// Log but don't fail the operation
		fmt.Printf("Warning: failed to sync config to database: %v\n", err)
	}

	return nil
}

// buildChannelConfig builds channel-specific configuration
func buildChannelConfig(channel, botToken, appToken string) map[string]interface{} {
	config := make(map[string]interface{})

	// Set botToken if provided (used by telegram, discord, slack, etc.)
	if botToken != "" {
		config["botToken"] = botToken
	}

	// Set appToken if provided (used by slack)
	if appToken != "" {
		config["appToken"] = appToken
	}

	return config
}

// readOpenClawConfig reads the openclaw.json config file from the pod
func readOpenClawConfig(ctx context.Context, namespace, podName string) (map[string]interface{}, error) {
	output, err := ExecInPod(ctx, namespace, podName, "openclaw",
		[]string{"cat", "/home/node/.openclaw/openclaw.json"})
	if err != nil {
		// If file doesn't exist, return empty config
		if strings.Contains(err.Error(), "No such file") {
			return make(map[string]interface{}), nil
		}
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(output), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return config, nil
}

// writeOpenClawConfig writes the openclaw.json config file to the pod
func writeOpenClawConfig(ctx context.Context, namespace, podName string, config map[string]interface{}) error {
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	command := []string{"sh", "-c", fmt.Sprintf("cat > /home/node/.openclaw/openclaw.json << 'EOFCONFIG'\n%s\nEOFCONFIG", string(configJSON))}

	_, err = ExecInPod(ctx, namespace, podName, "openclaw", command)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// ApproveChannelPairing approves a channel pairing request using the pairing code
// Example: openclaw pairing approve telegram JDB55KTQ
func ApproveChannelPairing(ctx context.Context, botID, channel, code string) (string, error) {
	namespace := GetNamespace()

	podName, err := WaitForPodReady(ctx, botID, 30)
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	// Execute pairing approve command
	command := []string{"node", "/app/openclaw.mjs", "pairing", "approve", channel, code}

	output, err := ExecInPod(ctx, namespace, podName, "openclaw", command)
	if err != nil {
		return "", fmt.Errorf("failed to approve pairing: %w", err)
	}

	return strings.TrimSpace(output), nil
}

// ChannelPairingResponse represents the response from openclaw pairing list
type ChannelPairingResponse struct {
	Channel  string                   `json:"channel"`
	Requests []ChannelPairingListItem `json:"requests"`
}

// ChannelPairingListItem represents a single pairing request
type ChannelPairingListItem struct {
	ID         string                 `json:"id"`
	Code       string                 `json:"code"`
	CreatedAt  string                 `json:"createdAt"`
	LastSeenAt string                 `json:"lastSeenAt"`
	Meta       map[string]interface{} `json:"meta"`
}

// ChannelPairedUser represents a paired user
type ChannelPairedUser struct {
	ID       string                 `json:"id"`
	Username string                 `json:"username,omitempty"`
	Meta     map[string]interface{} `json:"meta,omitempty"`
}

// ListChannelPairingRequests lists pending pairing requests for a channel
// Example: openclaw pairing list telegram --json
func ListChannelPairingRequests(ctx context.Context, botID, channel string) (*ChannelPairingResponse, error) {
	namespace := GetNamespace()

	podName, err := WaitForPodReady(ctx, botID, 30)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	// Execute pairing list command
	command := []string{"node", "/app/openclaw.mjs", "pairing", "list", channel, "--json"}

	output, err := ExecInPod(ctx, namespace, podName, "openclaw", command)
	if err != nil {
		return nil, fmt.Errorf("failed to list pairing requests: %w", err)
	}

	// Parse JSON output
	var response ChannelPairingResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// RevokeChannelPairing revokes a channel pairing for a user
// This removes the user from the allowFrom list in config
func RevokeChannelPairing(ctx context.Context, botID, channel, userID string) (string, error) {
	namespace := GetNamespace()

	podName, err := WaitForPodReady(ctx, botID, 30)
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	// First try the CLI command
	command := []string{"node", "/app/openclaw.mjs", "pairing", "revoke", channel, userID}
	output, err := ExecInPod(ctx, namespace, podName, "openclaw", command)
	if err == nil {
		return strings.TrimSpace(output), nil
	}

	// If CLI command fails, remove from config allowFrom list
	existingConfig, err := readOpenClawConfig(ctx, namespace, podName)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}

	// Get channels config
	channels, ok := existingConfig["channels"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no channels configured")
	}

	// Get specific channel config
	channelConfig, ok := channels[channel].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("channel %s not configured", channel)
	}

	// Remove from allowFrom list
	if allowFrom, ok := channelConfig["allowFrom"].([]interface{}); ok {
		var newAllowFrom []interface{}
		for _, v := range allowFrom {
			if str, ok := v.(string); ok && str != userID {
				newAllowFrom = append(newAllowFrom, v)
			} else if num, ok := v.(float64); ok && fmt.Sprintf("%.0f", num) != userID {
				newAllowFrom = append(newAllowFrom, v)
			}
		}
		channelConfig["allowFrom"] = newAllowFrom
	}

	// Write updated config
	if err := writeOpenClawConfig(ctx, namespace, podName, existingConfig); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	return "user removed from allowFrom list", nil
}

// GetChannelPairedUsers gets the list of paired users for a channel
// Tries multiple methods: sessions command, channels status, and config allowFrom
func GetChannelPairedUsers(ctx context.Context, botID, channel string) ([]ChannelPairedUser, error) {
	namespace := GetNamespace()

	podName, err := WaitForPodReady(ctx, botID, 30)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	var users []ChannelPairedUser

	// Try 1: Use sessions command to get active sessions
	command := []string{"node", "/app/openclaw.mjs", "sessions", "list", "--json"}
	output, err := ExecInPod(ctx, namespace, podName, "openclaw", command)
	if err == nil {
		// Parse sessions and filter by channel
		var sessions []map[string]interface{}
		if json.Unmarshal([]byte(output), &sessions) == nil {
			for _, s := range sessions {
				if ch, ok := s["channel"].(string); ok && ch == channel {
					user := ChannelPairedUser{
						Meta: make(map[string]interface{}),
					}
					if id, ok := s["userId"].(string); ok {
						user.ID = id
					} else if id, ok := s["userId"].(float64); ok {
						user.ID = fmt.Sprintf("%.0f", id)
					}
					if username, ok := s["username"].(string); ok {
						user.Username = username
					}
					// Copy other metadata
					for k, v := range s {
						if k != "userId" && k != "username" && k != "channel" {
							user.Meta[k] = v
						}
					}
					if user.ID != "" || user.Username != "" {
						users = append(users, user)
					}
				}
			}
			if len(users) > 0 {
				return users, nil
			}
		}
	}

	// Try 2: Use channels status command
	command = []string{"node", "/app/openclaw.mjs", "channels", "status", channel, "--json"}
	output, err = ExecInPod(ctx, namespace, podName, "openclaw", command)
	if err == nil {
		var status map[string]interface{}
		if json.Unmarshal([]byte(output), &status) == nil {
			// Look for paired users in status
			if paired, ok := status["pairedUsers"].([]interface{}); ok {
				for _, p := range paired {
					if userMap, ok := p.(map[string]interface{}); ok {
						user := ChannelPairedUser{Meta: userMap}
						if id, ok := userMap["id"].(string); ok {
							user.ID = id
						} else if id, ok := userMap["id"].(float64); ok {
							user.ID = fmt.Sprintf("%.0f", id)
						}
						if username, ok := userMap["username"].(string); ok {
							user.Username = username
						}
						users = append(users, user)
					}
				}
			}
			if len(users) > 0 {
				return users, nil
			}
		}
	}

	// Try 3: Read from config allowFrom list (pre-authorized users)
	existingConfig, err := readOpenClawConfig(ctx, namespace, podName)
	if err != nil {
		return users, nil // Return empty, don't fail
	}

	// Get channels config
	channels, ok := existingConfig["channels"].(map[string]interface{})
	if !ok {
		return users, nil
	}

	// Get specific channel config
	channelConfig, ok := channels[channel].(map[string]interface{})
	if !ok {
		return users, nil
	}

	// Get allowFrom list
	if allowFrom, ok := channelConfig["allowFrom"].([]interface{}); ok {
		for _, v := range allowFrom {
			var user ChannelPairedUser
			switch val := v.(type) {
			case string:
				if strings.HasPrefix(val, "@") {
					user.Username = val[1:]
				} else {
					user.ID = val
				}
			case float64:
				user.ID = fmt.Sprintf("%.0f", val)
			}
			if user.ID != "" || user.Username != "" {
				user.Meta = map[string]interface{}{"source": "allowFrom"}
				users = append(users, user)
			}
		}
	}

	return users, nil
}
