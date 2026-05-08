package k8s

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// getShortID returns first 8 chars of botID to keep K8s names under 63 chars
func getShortID(botID string) string {
	if len(botID) > 8 {
		return botID[:8]
	}
	return botID
}

// imagePullPolicy returns PullAlways for "latest" tag, PullIfNotPresent otherwise.
// In local dev mode, always use PullIfNotPresent to allow local images.
func imagePullPolicy(image string) corev1.PullPolicy {
	if viper.GetBool("kubernetes.local_dev") {
		return corev1.PullIfNotPresent
	}
	// "foo:latest", "foo" (no tag defaults to latest), or "foo:Latest"
	if !strings.Contains(image, ":") || strings.HasSuffix(strings.ToLower(image), ":latest") {
		return corev1.PullAlways
	}
	return corev1.PullIfNotPresent
}

func GetDeploymentName(botID string) string {
	return fmt.Sprintf("oc-%s", getShortID(botID))
}

func GetServiceName(botID string) string {
	return fmt.Sprintf("oc-%s-svc", getShortID(botID))
}

// ModelProviderConfig holds a single model provider configuration
type ModelProviderConfig struct {
	Name       string             // Provider name (anthropic, openai, minimax)
	BaseURL    string             // Base URL for the provider API
	APIKey     string             // API key
	Auth       string             // Auth mode: "api-key" (default), "bearer", etc.
	AuthHeader bool               // Whether to send API key in Authorization header
	API        string             // API format: "anthropic-messages" (default), "openai-completions", etc.
	Models     []ModelConfigEntry // Model configurations
}

// ModelConfigEntry holds a single model configuration
type ModelConfigEntry struct {
	ID            string
	Name          string
	Reasoning     bool
	Input         []string
	ContextWindow int
	MaxTokens     int
}

// AgentDefaultsConfig holds agent default configuration
type AgentDefaultsConfig struct {
	PrimaryModel  string // e.g., "anthropic/claude-sonnet-4-20250514"
	FallbackModel string // e.g., "anthropic/claude-haiku-4-5-20251001" - used when primary is unavailable
}

// BotConfig holds the configuration for a bot
type BotConfig struct {
	// Legacy single provider fields (kept for backward compatibility)
	Provider string // Provider key name in openclaw config (e.g., "anthropic", "minimax")
	Model    string
	APIKey   string
	BaseURL  string // For MiniMax or other Anthropic-compatible APIs
	Auth     string // Auth mode: "api-key" (default), "bearer", etc.
	API      string // API format: "anthropic-messages" (default), "openai-completions", etc.

	// Access token for CLI commands
	AccessToken string

	// Multi-provider support
	Providers     []ModelProviderConfig
	AgentDefaults *AgentDefaultsConfig

	// Channels configuration (telegram, slack, discord, etc.)
	Channels map[string]interface{}
}

// buildDeploymentSpec builds the full Deployment object for a bot.
// Shared by CreateDeployment and ReplaceDeployment to ensure consistency.
func buildDeploymentSpec(botID, userID string, config *BotConfig) *appsv1.Deployment {
	namespace := GetNamespace()
	deploymentName := GetDeploymentName(botID)

	// Get config values
	image := viper.GetString("openclaw.image")
	if image == "" {
		image = "openclaw/openclaw:latest"
	}
	gatewayPort := viper.GetInt32("openclaw.gateway_port")
	if gatewayPort == 0 {
		gatewayPort = 18789
	}
	pvcName := viper.GetString("storage.pvc_name")
	if pvcName == "" {
		pvcName = "openclaw-shared-data"
	}

	cpuLimit := viper.GetString("openclaw.cpu_limit")
	if cpuLimit == "" {
		cpuLimit = "500m"
	}
	memoryLimit := viper.GetString("openclaw.memory_limit")
	if memoryLimit == "" {
		memoryLimit = "512Mi"
	}
	cpuRequest := viper.GetString("openclaw.cpu_request")
	if cpuRequest == "" {
		cpuRequest = "100m"
	}
	memoryRequest := viper.GetString("openclaw.memory_request")
	if memoryRequest == "" {
		memoryRequest = "128Mi"
	}
	nodeMaxOldSpaceSize := viper.GetInt("openclaw.node_max_old_space_size")
	if nodeMaxOldSpaceSize == 0 {
		nodeMaxOldSpaceSize = 3072
	}
	ephemeralStorageLimit := viper.GetString("openclaw.ephemeral_storage_limit")
	if ephemeralStorageLimit == "" {
		ephemeralStorageLimit = "1Gi"
	}
	imagePullSecret := viper.GetString("openclaw.image_pull_secret")

	labels := map[string]string{
		"app":     "openclaw",
		"bot-id":  botID,
		"user-id": userID,
	}

	replicas := int32(1)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: func() []corev1.LocalObjectReference {
						if imagePullSecret != "" {
							return []corev1.LocalObjectReference{{Name: imagePullSecret}}
						}
						return nil
					}(),
					InitContainers: func() []corev1.Container {
						initCmd := "chown -R 1000:1000 /data && chmod -R 755 /data"
						mounts := []corev1.VolumeMount{
							{
								Name:      "data",
								MountPath: "/data",
								SubPath:   botID,
							},
						}
						if ChatClawEnabled() {
							initCmd += " && chown -R 1000:1000 /chatclaw-data && chmod -R 755 /chatclaw-data"
							mounts = append(mounts, corev1.VolumeMount{
								Name:      "data",
								MountPath: "/chatclaw-data",
								SubPath:   botID + "/chatclaw-data",
							})
						}
						return []corev1.Container{
							{
								Name:         "init-permissions",
								Image:        "alpine:3.19",
								Command:      []string{"sh", "-c", initCmd},
								VolumeMounts: mounts,
								SecurityContext: &corev1.SecurityContext{
									RunAsUser:                func() *int64 { v := int64(0); return &v }(),
									AllowPrivilegeEscalation: func() *bool { v := false; return &v }(),
									ReadOnlyRootFilesystem:   func() *bool { v := true; return &v }(),
								},
							},
						}
					}(),
					Containers: func() []corev1.Container {
						containers := []corev1.Container{
							{
								Name:            "openclaw",
								Image:           image,
								ImagePullPolicy: imagePullPolicy(image),
								SecurityContext: &corev1.SecurityContext{
									RunAsUser:                func() *int64 { v := int64(1000); return &v }(),
									RunAsGroup:               func() *int64 { v := int64(1000); return &v }(),
									AllowPrivilegeEscalation: func() *bool { v := false; return &v }(),
								},
								Ports: []corev1.ContainerPort{
									{
										Name:          "gateway",
										ContainerPort: gatewayPort,
										Protocol:      corev1.ProtocolTCP,
									},
								},
								Command: func() []string {
									if config != nil && config.AccessToken != "" {
										configJSON := buildOpenClawConfig(config, true)
										// Only write config if it doesn't exist yet (first start).
										// On restart, the PVC already has the live config (possibly modified
										// by user via OpenClaw UI), so we must not overwrite it.
										return []string{"sh", "-c", fmt.Sprintf(`if [ ! -f /home/node/.openclaw/openclaw.json ]; then
cat > /home/node/.openclaw/openclaw.json << 'EOFCONFIG'
%s
EOFCONFIG
fi
# Patch config: clean up invalid keys and ensure controlUi is set
if command -v node > /dev/null 2>&1 && [ -f /home/node/.openclaw/openclaw.json ]; then
  node -e "
    const fs = require('fs');
    const f = '/home/node/.openclaw/openclaw.json';
    try {
      const c = JSON.parse(fs.readFileSync(f, 'utf8'));
      let changed = false;
      if (c.gateway && c.gateway.auth && c.gateway.auth.scopes) {
        delete c.gateway.auth.scopes;
        changed = true;
      }
      if (c.gateway) {
        const wantUi = { allowedOrigins: ['*'], dangerouslyDisableDeviceAuth: true };
        if (!c.gateway.controlUi || JSON.stringify(c.gateway.controlUi) !== JSON.stringify(wantUi)) {
          c.gateway.controlUi = wantUi;
          changed = true;
        }
        if (!c.gateway.http || !c.gateway.http.endpoints || !c.gateway.http.endpoints.chatCompletions) {
          c.gateway.http = { endpoints: { chatCompletions: { enabled: true } } };
          changed = true;
        }
      }
      if (changed) fs.writeFileSync(f, JSON.stringify(c, null, 2));
    } catch(e) {}
  " 2>/dev/null
fi
# Seed pre-installed plugins on first boot. Use -n so user data on an
# existing PVC is never overwritten.
if [ ! -f /home/node/.openclaw/.openclaw-init-done ] && [ -d /opt/openclaw-init ]; then
  cp -an /opt/openclaw-init/. /home/node/.openclaw/
  touch /home/node/.openclaw/.openclaw-init-done
fi
# Auto-migrate config schema between openclaw versions (idempotent no-op when valid)
openclaw doctor --fix 2>/dev/null || true
exec openclaw gateway --port %d --bind lan --allow-unconfigured --dev`, configJSON, gatewayPort)}
									}
									return []string{"openclaw", "gateway", "--port", fmt.Sprintf("%d", gatewayPort), "--bind", "lan", "--allow-unconfigured", "--dev"}
								}(),
								Env: func() []corev1.EnvVar {
									envs := []corev1.EnvVar{
										{
											Name:  "NODE_OPTIONS",
											Value: fmt.Sprintf("--max-old-space-size=%d", nodeMaxOldSpaceSize),
										},
									}
									if config != nil {
										if config.APIKey != "" {
											envs = append(envs, corev1.EnvVar{
												Name:  "ANTHROPIC_API_KEY",
												Value: config.APIKey,
											})
										}
										if config.Model != "" {
											envs = append(envs, corev1.EnvVar{
												Name:  "CLAUDE_MODEL",
												Value: config.Model,
											})
										}
										if config.BaseURL != "" {
											envs = append(envs, corev1.EnvVar{
												Name:  "ANTHROPIC_BASE_URL",
												Value: config.BaseURL,
											})
										}
									}
									return envs
								}(),
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "data",
										MountPath: "/home/node/.openclaw",
										SubPath:   botID,
									},
								},
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:              resource.MustParse(cpuLimit),
										corev1.ResourceMemory:           resource.MustParse(memoryLimit),
										corev1.ResourceEphemeralStorage: resource.MustParse(ephemeralStorageLimit),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse(cpuRequest),
										corev1.ResourceMemory: resource.MustParse(memoryRequest),
									},
								},
								LivenessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										TCPSocket: &corev1.TCPSocketAction{
											Port: intstr.FromInt32(gatewayPort),
										},
									},
									InitialDelaySeconds: 180,
									PeriodSeconds:       30,
									FailureThreshold:    5,
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										TCPSocket: &corev1.TCPSocketAction{
											Port: intstr.FromInt32(gatewayPort),
										},
									},
									InitialDelaySeconds: 60,
									PeriodSeconds:       10,
									FailureThreshold:    15,
								},
							},
						}

						// Add ChatClaw sidecar when enabled
						if ChatClawEnabled() {
							ccImage := viper.GetString("chatclaw.image")
							ccPort := ChatClawPort()

							ccCPULimit := viper.GetString("chatclaw.cpu_limit")
							if ccCPULimit == "" {
								ccCPULimit = "1000m"
							}
							ccMemoryLimit := viper.GetString("chatclaw.memory_limit")
							if ccMemoryLimit == "" {
								ccMemoryLimit = "1Gi"
							}
							ccCPURequest := viper.GetString("chatclaw.cpu_request")
							if ccCPURequest == "" {
								ccCPURequest = "100m"
							}
							ccMemoryRequest := viper.GetString("chatclaw.memory_request")
							if ccMemoryRequest == "" {
								ccMemoryRequest = "256Mi"
							}

							containers = append(containers, corev1.Container{
								Name:            "chatclaw",
								Image:           ccImage,
								ImagePullPolicy: imagePullPolicy(ccImage),
								SecurityContext: &corev1.SecurityContext{
									RunAsUser:                func() *int64 { v := int64(1000); return &v }(),
									RunAsGroup:               func() *int64 { v := int64(1000); return &v }(),
									AllowPrivilegeEscalation: func() *bool { v := false; return &v }(),
								},
								Ports: []corev1.ContainerPort{
									{
										Name:          "chatclaw",
										ContainerPort: ccPort,
										Protocol:      corev1.ProtocolTCP,
									},
								},
								Env: func() []corev1.EnvVar {
									dbBackend := viper.GetString("chatclaw.db_backend")
									if dbBackend == "" {
										dbBackend = "drizzle"
									}
									dataDir := viper.GetString("chatclaw.data_dir")
									if dataDir == "" {
										dataDir = "/data"
									}
									authEnabled := viper.GetString("chatclaw.auth_enabled")
									if authEnabled == "" {
										authEnabled = "false"
									}
									multiCompany := viper.GetString("chatclaw.multi_company")
									if multiCompany == "" {
										multiCompany = "false"
									}
									return []corev1.EnvVar{
										{Name: "PORT", Value: fmt.Sprintf("%d", ccPort)},
										{Name: "HOSTNAME", Value: "0.0.0.0"},
										{Name: "NODE_ENV", Value: "production"},
										{Name: "NEXT_TELEMETRY_DISABLED", Value: "1"},
										{Name: "DB_BACKEND", Value: dbBackend},
										{Name: "CHATCLAW_DATA_DIR", Value: dataDir},
										{Name: "AUTH_ENABLED", Value: authEnabled},
										{Name: "MULTI_COMPANY", Value: multiCompany},
										// Set HOME so ~/.openclaw resolves to the shared volume
										{Name: "HOME", Value: "/home/node"},
									}
								}(),
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "data",
										MountPath: "/home/node/.openclaw",
										SubPath:   botID,
									},
									{
										Name:      "data",
										MountPath: "/data",
										SubPath:   botID + "/chatclaw-data",
									},
								},
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse(ccCPULimit),
										corev1.ResourceMemory: resource.MustParse(ccMemoryLimit),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse(ccCPURequest),
										corev1.ResourceMemory: resource.MustParse(ccMemoryRequest),
									},
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										TCPSocket: &corev1.TCPSocketAction{
											Port: intstr.FromInt32(ccPort),
										},
									},
									InitialDelaySeconds: 10,
									PeriodSeconds:       10,
									FailureThreshold:    10,
								},
								LivenessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										TCPSocket: &corev1.TCPSocketAction{
											Port: intstr.FromInt32(ccPort),
										},
									},
									InitialDelaySeconds: 30,
									PeriodSeconds:       30,
									FailureThreshold:    3,
								},
							})
						}

						return containers
					}(),
					Volumes: func() []corev1.Volume {
						vols := []corev1.Volume{
							{
								Name: "data",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: pvcName,
									},
								},
							},
						}
						return vols
					}(),
				},
			},
		},
	}
}

func CreateDeployment(ctx context.Context, botID, userID, accessToken string, config *BotConfig) error {
	client := GetClient()
	namespace := GetNamespace()

	deployment := buildDeploymentSpec(botID, userID, config)

	_, err := client.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Deployment exists (maybe scaled to 0 from a previous stop).
			// Update the full spec and ensure replicas=1.
			return ReplaceDeployment(ctx, botID, userID, accessToken, config)
		}
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	return nil
}

// ReplaceDeployment updates the full deployment spec and triggers a rolling update.
// Unlike RestartDeployment (annotation-only), this picks up all spec changes
// including new sidecar containers, image updates, resource changes, etc.
func ReplaceDeployment(ctx context.Context, botID, userID, accessToken string, config *BotConfig) error {
	client := GetClient()
	namespace := GetNamespace()

	deployment := buildDeploymentSpec(botID, userID, config)

	// Add restart annotation to ensure rollout even if spec is identical
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = metav1.Now().Format("2006-01-02T15:04:05Z07:00")

	_, err := client.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
}

func DeleteDeployment(ctx context.Context, botID string) error {
	client := GetClient()
	namespace := GetNamespace()
	deploymentName := GetDeploymentName(botID)

	err := client.AppsV1().Deployments(namespace).Delete(ctx, deploymentName, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

func GetDeploymentStatus(ctx context.Context, botID string) (bool, error) {
	client := GetClient()
	namespace := GetNamespace()
	deploymentName := GetDeploymentName(botID)

	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get deployment: %w", err)
	}

	return deployment.Status.ReadyReplicas > 0, nil
}

// DeploymentStatusInfo holds detailed deployment status
type DeploymentStatusInfo struct {
	Status          string `json:"status"`           // ready, updating, starting, not_ready, not_found
	ReadyReplicas   int32  `json:"ready_replicas"`   // Number of ready pods
	DesiredReplicas int32  `json:"desired_replicas"` // Desired number of pods
	UpdatedReplicas int32  `json:"updated_replicas"` // Number of pods with updated spec
}

// GetDeploymentStatusInfo returns detailed deployment status
func GetDeploymentStatusInfo(ctx context.Context, botID string) (*DeploymentStatusInfo, error) {
	client := GetClient()
	namespace := GetNamespace()
	deploymentName := GetDeploymentName(botID)

	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return &DeploymentStatusInfo{Status: "not_found"}, nil
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	info := &DeploymentStatusInfo{
		ReadyReplicas:   deployment.Status.ReadyReplicas,
		DesiredReplicas: *deployment.Spec.Replicas,
		UpdatedReplicas: deployment.Status.UpdatedReplicas,
	}

	// Determine status
	if *deployment.Spec.Replicas == 0 {
		info.Status = "stopped"
	} else if deployment.Status.ReadyReplicas == 0 {
		info.Status = "starting"
	} else if deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
		// Rolling update in progress
		info.Status = "updating"
	} else if deployment.Status.ReadyReplicas < *deployment.Spec.Replicas {
		info.Status = "not_ready"
	} else {
		info.Status = "ready"
	}

	return info, nil
}

// DeploymentExists checks if a deployment exists for the given bot
func DeploymentExists(ctx context.Context, botID string) (bool, error) {
	client := GetClient()
	namespace := GetNamespace()
	deploymentName := GetDeploymentName(botID)

	_, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get deployment: %w", err)
	}

	return true, nil
}

func RestartDeployment(ctx context.Context, botID string) error {
	client := GetClient()
	namespace := GetNamespace()
	deploymentName := GetDeploymentName(botID)

	// Get current deployment
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Add/update restart annotation to trigger rollout
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = metav1.Now().Format("2006-01-02T15:04:05Z07:00")

	_, err = client.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
}

// UpdateDeploymentImage updates the container image and triggers a rolling update
func UpdateDeploymentImage(ctx context.Context, botID, newImage string) error {
	client := GetClient()
	namespace := GetNamespace()
	deploymentName := GetDeploymentName(botID)

	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Update image for the openclaw container
	for i := range deployment.Spec.Template.Spec.Containers {
		if deployment.Spec.Template.Spec.Containers[i].Name == "openclaw" {
			deployment.Spec.Template.Spec.Containers[i].Image = newImage
			deployment.Spec.Template.Spec.Containers[i].ImagePullPolicy = imagePullPolicy(newImage)
			break
		}
	}

	// Add restart annotation to trigger rollout
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = metav1.Now().Format("2006-01-02T15:04:05Z07:00")

	_, err = client.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment image: %w", err)
	}

	return nil
}

// GetDeploymentImage returns the current openclaw container image for a bot
func GetDeploymentImage(ctx context.Context, botID string) (string, error) {
	client := GetClient()
	namespace := GetNamespace()
	deploymentName := GetDeploymentName(botID)

	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get deployment: %w", err)
	}

	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == "openclaw" {
			return c.Image, nil
		}
	}

	return "", fmt.Errorf("openclaw container not found")
}

// UpdateDeploymentConfig updates the deployment with new config and triggers rolling update
func UpdateDeploymentConfig(ctx context.Context, botID, accessToken string, config *BotConfig) error {
	client := GetClient()
	namespace := GetNamespace()
	deploymentName := GetDeploymentName(botID)

	// Get current deployment
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Get config values
	gatewayPort := viper.GetInt32("openclaw.gateway_port")
	if gatewayPort == 0 {
		gatewayPort = 18789
	}
	nodeMaxOldSpaceSize := viper.GetInt("openclaw.node_max_old_space_size")
	if nodeMaxOldSpaceSize == 0 {
		nodeMaxOldSpaceSize = 3072
	}

	// Update container - write config file before starting gateway
	for i := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[i]
		if container.Name == "openclaw" {
			// Update command - write full config (including models) before starting gateway
			if config != nil && config.AccessToken != "" {
				// Use full config to preserve models section
				configJSON := buildOpenClawConfig(config, true)
				container.Command = []string{"sh", "-c", fmt.Sprintf(`cat > /home/node/.openclaw/openclaw.json << 'EOFCONFIG'
%s
EOFCONFIG
node /app/openclaw.mjs gateway --port %d --bind lan --allow-unconfigured --dev`, configJSON, gatewayPort)}
			} else {
				container.Command = []string{"node", "/app/openclaw.mjs", "gateway", "--port", fmt.Sprintf("%d", gatewayPort), "--bind", "lan", "--allow-unconfigured", "--dev"}
			}

			// Update env vars
			newEnvs := []corev1.EnvVar{
				{Name: "NODE_OPTIONS", Value: fmt.Sprintf("--max-old-space-size=%d", nodeMaxOldSpaceSize)},
			}
			if config != nil {
				if config.APIKey != "" {
					newEnvs = append(newEnvs, corev1.EnvVar{Name: "ANTHROPIC_API_KEY", Value: config.APIKey})
				}
				if config.Model != "" {
					newEnvs = append(newEnvs, corev1.EnvVar{Name: "CLAUDE_MODEL", Value: config.Model})
				}
				if config.BaseURL != "" {
					newEnvs = append(newEnvs, corev1.EnvVar{Name: "ANTHROPIC_BASE_URL", Value: config.BaseURL})
				}
			}
			container.Env = newEnvs
			break
		}
	}

	// Add restart annotation to trigger rollout
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = metav1.Now().Format("2006-01-02T15:04:05Z07:00")

	// Update deployment
	_, err = client.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
}
