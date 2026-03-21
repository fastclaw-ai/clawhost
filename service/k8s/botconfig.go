package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// WriteConfigToBot writes the openclaw.json config file to the bot's pod
// If forceSetDefaultModel is true, always set agents.defaults.model.primary
// This function MERGES with existing config to preserve channels and other settings
func WriteConfigToBot(ctx context.Context, botID string, config *BotConfig, forceSetDefaultModel bool) error {
	if config == nil {
		return nil
	}

	namespace := GetNamespace()

	// Wait for pod to be ready and get pod name
	podName, err := waitForPodReady(ctx, botID, 60) // 60 seconds timeout
	if err != nil {
		return fmt.Errorf("failed to wait for pod ready: %w", err)
	}

	// Read existing config to preserve channels and other settings
	existingConfig, err := readExistingConfig(ctx, namespace, podName)
	if err != nil {
		// If can't read, start with empty config
		existingConfig = make(map[string]interface{})
	}

	// Determine whether to set default model
	setDefaultModel := forceSetDefaultModel
	if !forceSetDefaultModel {
		// Check if user has already configured a default model
		if agents, ok := existingConfig["agents"].(map[string]interface{}); ok {
			if defaults, ok := agents["defaults"].(map[string]interface{}); ok {
				if model, ok := defaults["model"].(map[string]interface{}); ok {
					if _, ok := model["primary"]; ok {
						setDefaultModel = false
					}
				}
			}
		}
	}

	// Merge new config into existing config
	mergedConfig := mergeConfigForModels(existingConfig, config, setDefaultModel)

	// Marshal to JSON
	configJSON, err := json.MarshalIndent(mergedConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write config file to OpenClaw's config directory
	command := []string{"sh", "-c", fmt.Sprintf("cat > /home/node/.openclaw/openclaw.json << 'EOFCONFIG'\n%s\nEOFCONFIG", string(configJSON))}

	_, err = ExecInPod(ctx, namespace, podName, "openclaw", command)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// ReadBotConfig reads the openclaw.json config from a running bot's pod
func ReadBotConfig(ctx context.Context, botID string) (map[string]interface{}, error) {
	namespace := GetNamespace()
	podName, err := waitForPodReady(ctx, botID, 10)
	if err != nil {
		return nil, fmt.Errorf("pod not ready: %w", err)
	}
	return readExistingConfig(ctx, namespace, podName)
}

// WriteBotConfig writes a full openclaw.json config to a running bot's pod
func WriteBotConfig(ctx context.Context, botID string, config map[string]interface{}) error {
	namespace := GetNamespace()
	podName, err := waitForPodReady(ctx, botID, 10)
	if err != nil {
		return fmt.Errorf("pod not ready: %w", err)
	}

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

// readExistingConfig reads the existing openclaw.json config from the pod
func readExistingConfig(ctx context.Context, namespace, podName string) (map[string]interface{}, error) {
	output, err := ExecInPod(ctx, namespace, podName, "openclaw",
		[]string{"sh", "-c", "cat /home/node/.openclaw/openclaw.json 2>/dev/null || echo '{}'"})
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(output), &config); err != nil {
		return nil, err
	}

	return config, nil
}

// mergeConfigForModels merges model/gateway config into existing config, preserving channels
func mergeConfigForModels(existing map[string]interface{}, config *BotConfig, setDefaultModel bool) map[string]interface{} {
	// Build gateway config
	gatewayPort := getGatewayPort()
	trustedProxies := viper.GetStringSlice("openclaw.trusted_proxies")
	if len(trustedProxies) == 0 {
		trustedProxies = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8"}
	}

	// Build auth config - preserve existing gateway.auth if present, otherwise use token auth
	var authConfig map[string]interface{}
	if existingGateway, ok := existing["gateway"].(map[string]interface{}); ok {
		if existingAuth, ok := existingGateway["auth"].(map[string]interface{}); ok {
			// Preserve existing auth config
			authConfig = existingAuth
		}
	}
	// If no existing auth or access token provided, use token auth
	if authConfig == nil {
		authConfig = map[string]interface{}{
			"mode":  "token",
			"token": config.AccessToken,
		}
	}
	// Clean up invalid keys that OpenClaw doesn't recognize
	delete(authConfig, "scopes")

	// Merge into existing gateway config to preserve key ordering and avoid
	// unnecessary config change detection (which triggers gateway self-restart).
	gateway, _ := existing["gateway"].(map[string]interface{})
	if gateway == nil {
		gateway = make(map[string]interface{})
	}
	gateway["port"] = gatewayPort
	gateway["mode"] = "local"
	gateway["bind"] = "lan"
	gateway["auth"] = authConfig
	if gateway["tailscale"] == nil {
		gateway["tailscale"] = map[string]interface{}{
			"mode":        "off",
			"resetOnExit": false,
		}
	}
	gateway["trustedProxies"] = trustedProxies
	if gateway["controlUi"] == nil {
		gateway["controlUi"] = map[string]interface{}{
			"dangerouslyDisableDeviceAuth": true,
			"allowedOrigins":              []string{"*"},
		}
	}
	if gateway["http"] == nil {
		gateway["http"] = map[string]interface{}{
			"endpoints": map[string]interface{}{
				"chatCompletions": map[string]interface{}{
					"enabled": true,
				},
			},
		}
	}
	existing["gateway"] = gateway

	// Build models config
	providers := buildProvidersMap(config)
	existing["models"] = map[string]interface{}{
		"mode":      "merge",
		"providers": providers,
	}

	// Set default model if needed
	if setDefaultModel {
		defaultModel := getDefaultModelFromConfig(config)
		if config.AgentDefaults != nil && config.AgentDefaults.PrimaryModel != "" {
			defaultModel = config.AgentDefaults.PrimaryModel
		}
		if defaultModel != "" {
			defaults := map[string]interface{}{
				"model": map[string]interface{}{
					"primary": defaultModel,
				},
			}
			// Set fallback model if provided
			if config.AgentDefaults != nil && config.AgentDefaults.FallbackModel != "" {
				defaults["models"] = map[string]interface{}{
					"fallback": map[string]interface{}{
						"alias": config.AgentDefaults.FallbackModel,
					},
				}
			}
			existing["agents"] = map[string]interface{}{
				"defaults": defaults,
			}
		}
	}

	// Merge channels from config if provided
	// This allows setting channels during bot creation or update
	if len(config.Channels) > 0 {
		existingChannels, _ := existing["channels"].(map[string]interface{})
		if existingChannels == nil {
			existingChannels = make(map[string]interface{})
		}
		// Merge each channel from config into existing channels
		for channelName, channelConfig := range config.Channels {
			existingChannels[channelName] = channelConfig
		}
		existing["channels"] = existingChannels
	}
	// If no channels in config, preserve existing channels (don't touch them)

	return existing
}

// buildProvidersMap builds the providers map from BotConfig
func buildProvidersMap(config *BotConfig) map[string]interface{} {
	providers := make(map[string]interface{})

	if len(config.Providers) > 0 {
		for _, p := range config.Providers {
			providerObj := map[string]interface{}{
				"baseUrl":    p.BaseURL,
				"apiKey":     p.APIKey,
				"auth":       getAuthOrDefault(p.Auth),
				"authHeader": p.AuthHeader,
				"api":        getAPIOrDefault(p.API, p.Name),
			}
			if len(p.Models) > 0 {
				models := make([]map[string]interface{}, len(p.Models))
				for i, m := range p.Models {
					modelObj := map[string]interface{}{
						"id":            m.ID,
						"name":          m.Name,
						"reasoning":     m.Reasoning,
						"contextWindow": m.ContextWindow,
						"maxTokens":     m.MaxTokens,
					}
					if len(m.Input) > 0 {
						modelObj["input"] = m.Input
					} else {
						modelObj["input"] = []string{"text"}
					}
					if m.Name == "" {
						modelObj["name"] = m.ID
					}
					if m.ContextWindow == 0 {
						modelObj["contextWindow"] = 200000
					}
					if m.MaxTokens == 0 {
						modelObj["maxTokens"] = 8192
					}
					models[i] = modelObj
				}
				providerObj["models"] = models
			}
			providers[p.Name] = providerObj
		}
	} else {
		// Legacy single provider
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		model := getDefaultModel(config.Model)
		providerName := getProviderName(config.Provider, baseURL)

		providers[providerName] = map[string]interface{}{
			"baseUrl": baseURL,
			"apiKey":  config.APIKey,
			"auth":    getAuthOrDefault(config.Auth),
			"api":     getAPIOrDefault(config.API, providerName),
			"models": []map[string]interface{}{
				{
					"id":            model,
					"name":          model,
					"reasoning":     false,
					"input":         []string{"text"},
					"contextWindow": 200000,
					"maxTokens":     8192,
				},
			},
		}
	}

	return providers
}

// checkHasDefaultModel checks if user has already configured a default model
func checkHasDefaultModel(ctx context.Context, namespace, podName string) bool {
	// Read existing config and check for agents.defaults.model.primary
	output, err := ExecInPod(ctx, namespace, podName, "openclaw",
		[]string{"sh", "-c", "cat /home/node/.openclaw/openclaw.json 2>/dev/null || echo '{}'"})
	if err != nil {
		return false
	}
	// Simple check: if config contains "agents" with "defaults", user likely has configured it
	return strings.Contains(output, `"agents"`) && strings.Contains(output, `"primary"`)
}

// getProviderName returns the provider name, using explicit provider if set, otherwise inferred from baseURL
func getProviderName(provider, baseURL string) string {
	// Use explicit provider if specified
	if provider != "" {
		return provider
	}
	// Otherwise infer from baseURL
	if strings.Contains(baseURL, "minimax") {
		return "minimax"
	}
	return "anthropic"
}

// getDefaultModel returns the model or a default value
func getDefaultModel(model string) string {
	if model == "" {
		return "claude-sonnet-4-20250514"
	}
	return model
}

// getTrustedProxies returns the trusted proxy list from config
// Defaults to private network ranges if not configured
func getTrustedProxies() string {
	proxies := viper.GetStringSlice("openclaw.trusted_proxies")
	if len(proxies) == 0 {
		// Default: trust all private network ranges (RFC 1918)
		// This allows proxies from 10.x.x.x, 172.16-31.x.x, 192.168.x.x
		proxies = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8"}
	}
	// Format as JSON array
	quoted := make([]string, len(proxies))
	for i, p := range proxies {
		quoted[i] = fmt.Sprintf(`"%s"`, p)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}


// getGatewayPort returns the gateway port from config
func getGatewayPort() int {
	port := viper.GetInt("openclaw.gateway_port")
	if port == 0 {
		return 18789
	}
	return port
}

// buildOpenClawConfig builds the openclaw.json configuration content
// setDefaultModel: if true, also sets agents.defaults.model.primary (for first-time setup)
func buildOpenClawConfig(config *BotConfig, setDefaultModel bool) string {
	// Build gateway section with password or token auth
	gatewayPort := getGatewayPort()
	trustedProxies := getTrustedProxies()

	// Build optional gateway parts
	var optionalParts string
	if trustedProxies != "" {
		optionalParts += fmt.Sprintf(",\n    \"trustedProxies\": %s", trustedProxies)
	}
	// Build controlUi section — always allow all origins since proxy handles auth
	controlUiParts := `"dangerouslyDisableDeviceAuth": true,
      "allowedOrigins": ["*"]`
	optionalParts += fmt.Sprintf(",\n    \"controlUi\": {\n      %s\n    }", controlUiParts)

	// Always enable HTTP chat completions endpoint
	optionalParts += `,
    "http": {
      "endpoints": {
        "chatCompletions": {
          "enabled": true
        }
      }
    }`

	// Build auth section using token auth with AccessToken
	authSection := fmt.Sprintf(`"auth": {
      "mode": "token",
      "token": "%s"
    }`, config.AccessToken)

	gatewaySection := fmt.Sprintf(`"gateway": {
    "port": %d,
    "mode": "local",
    "bind": "lan",
    %s,
    "tailscale": {
      "mode": "off",
      "resetOnExit": false
    }%s
  }`, gatewayPort, authSection, optionalParts)

	// Build providers section
	providersJSON := buildProvidersJSON(config)

	// Build channels section if present
	channelsSection := ""
	if len(config.Channels) > 0 {
		channelsJSON, err := json.MarshalIndent(config.Channels, "  ", "  ")
		if err == nil {
			channelsSection = fmt.Sprintf(",\n  \"channels\": %s", string(channelsJSON))
		}
	}

	// Determine default model
	defaultModel := getDefaultModelFromConfig(config)

	// Check if we have AgentDefaults set explicitly
	if config.AgentDefaults != nil && config.AgentDefaults.PrimaryModel != "" {
		defaultModel = config.AgentDefaults.PrimaryModel
	}

	if setDefaultModel && defaultModel != "" {
		// Build agents.defaults section
		agentsSection := fmt.Sprintf(`"agents": {
    "defaults": {
      "model": {
        "primary": "%s"
      }`, defaultModel)

		// Add fallback model if provided
		if config.AgentDefaults != nil && config.AgentDefaults.FallbackModel != "" {
			agentsSection += fmt.Sprintf(`,
      "models": {
        "fallback": {
          "alias": "%s"
        }
      }`, config.AgentDefaults.FallbackModel)
		}

		agentsSection += `
    }
  }`

		return fmt.Sprintf(`{
  %s,
  %s,
  "models": {
    "mode": "merge",
    "providers": %s
  }%s
}`, gatewaySection, agentsSection, providersJSON, channelsSection)
	}

	// Only add providers, don't change user's default model
	return fmt.Sprintf(`{
  %s,
  "models": {
    "mode": "merge",
    "providers": %s
  }%s
}`, gatewaySection, providersJSON, channelsSection)
}

// buildProvidersJSON builds the providers JSON object from BotConfig
func buildProvidersJSON(config *BotConfig) string {
	// If we have multiple providers configured, use them
	if len(config.Providers) > 0 {
		providers := make(map[string]interface{})
		for _, p := range config.Providers {
			providerObj := map[string]interface{}{
				"baseUrl":    p.BaseURL,
				"apiKey":     p.APIKey,
				"auth":       getAuthOrDefault(p.Auth),
				"authHeader": p.AuthHeader,
				"api":        getAPIOrDefault(p.API, p.Name),
			}
			if len(p.Models) > 0 {
				models := make([]map[string]interface{}, len(p.Models))
				for i, m := range p.Models {
					modelObj := map[string]interface{}{
						"id":            m.ID,
						"name":          m.Name,
						"reasoning":     m.Reasoning,
						"contextWindow": m.ContextWindow,
						"maxTokens":     m.MaxTokens,
					}
					if len(m.Input) > 0 {
						modelObj["input"] = m.Input
					} else {
						modelObj["input"] = []string{"text"}
					}
					if m.Name == "" {
						modelObj["name"] = m.ID
					}
					if m.ContextWindow == 0 {
						modelObj["contextWindow"] = 200000
					}
					if m.MaxTokens == 0 {
						modelObj["maxTokens"] = 8192
					}
					models[i] = modelObj
				}
				providerObj["models"] = models
			}
			providers[p.Name] = providerObj
		}
		jsonBytes, _ := json.MarshalIndent(providers, "    ", "  ")
		return string(jsonBytes)
	}

	// Fallback to legacy single provider
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	model := getDefaultModel(config.Model)
	providerName := getProviderName(config.Provider, baseURL)

	auth := getAuthOrDefault(config.Auth)
	api := getAPIOrDefault(config.API, providerName)

	return fmt.Sprintf(`{
      "%s": {
        "baseUrl": "%s",
        "apiKey": "%s",
        "auth": "%s",
        "api": "%s",
        "models": [
          {
            "id": "%s",
            "name": "%s",
            "reasoning": false,
            "input": ["text"],
            "contextWindow": 200000,
            "maxTokens": 8192
          }
        ]
      }
    }`, providerName, baseURL, config.APIKey, auth, api, model, model)
}

// getDefaultModelFromConfig returns the default model ID from config
func getDefaultModelFromConfig(config *BotConfig) string {
	// Check AgentDefaults first
	if config.AgentDefaults != nil && config.AgentDefaults.PrimaryModel != "" {
		return config.AgentDefaults.PrimaryModel
	}

	// If we have providers, use the first provider's first model
	if len(config.Providers) > 0 {
		p := config.Providers[0]
		if len(p.Models) > 0 {
			return fmt.Sprintf("%s/%s", p.Name, p.Models[0].ID)
		}
	}

	// Fallback to legacy fields
	if config.Model != "" {
		providerName := getProviderName(config.Provider, config.BaseURL)
		return fmt.Sprintf("%s/%s", providerName, config.Model)
	}

	// Default
	return "anthropic/claude-sonnet-4-20250514"
}

// getAuthOrDefault returns the auth value or default "api-key"
func getAuthOrDefault(auth string) string {
	if auth == "" {
		return "api-key"
	}
	return auth
}

// getAPIOrDefault returns the API value or default based on provider
func getAPIOrDefault(api, provider string) string {
	if api != "" {
		return api
	}
	// Default based on provider
	if provider == "openai" {
		return "openai-completions"
	}
	return "anthropic-messages"
}

// BuildGatewayConfig builds the minimal gateway config for initial startup
// This is used to write config before gateway starts (in container command)
func BuildGatewayConfig(config *BotConfig, port int32) string {
	trustedProxies := getTrustedProxies()

	// Build optional parts
	var optionalParts string
	if trustedProxies != "" {
		optionalParts += fmt.Sprintf(`,
    "trustedProxies": %s`, trustedProxies)
	}
	// Always allow all origins — proxy handles auth
	controlUiParts := `"dangerouslyDisableDeviceAuth": true,
      "allowedOrigins": ["*"]`
	optionalParts += fmt.Sprintf(`,
    "controlUi": {
      %s
    }`, controlUiParts)

	// Build auth section using token auth with AccessToken
	authSection := fmt.Sprintf(`"auth": {
      "mode": "token",
      "token": "%s"
    }`, config.AccessToken)

	return fmt.Sprintf(`{
  "gateway": {
    "port": %d,
    "mode": "local",
    "bind": "lan",
    %s,
    "tailscale": {
      "mode": "off",
      "resetOnExit": false
    }%s
  }
}`, port, authSection, optionalParts)
}
