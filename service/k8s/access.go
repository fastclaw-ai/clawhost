package k8s

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessPod is a pod that has the bot's PVC subpath mounted at MountPath.
// Obtain one via OpenAccess and ALWAYS call Close() when done.
type AccessPod struct {
	Namespace string
	Name      string
	Container string
	MountPath string // path inside the pod where the bot's data root lives
	BotID     string

	cleanup func()
}

// Close deletes the temp pod if OpenAccess spawned one. No-op for running bot pods.
func (ap *AccessPod) Close() {
	if ap != nil && ap.cleanup != nil {
		ap.cleanup()
	}
}

// AbsPath converts a path relative to the bot's data root to an absolute
// in-pod path. Rejects absolute paths and ".." escapes.
func (ap *AccessPod) AbsPath(rel string) (string, error) {
	if rel == "" || rel == "." {
		return ap.MountPath, nil
	}
	if path.IsAbs(rel) {
		return "", fmt.Errorf("path must be relative to bot data root, got %q", rel)
	}
	cleaned := path.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path escapes data root: %q", rel)
	}
	return path.Join(ap.MountPath, cleaned), nil
}

// OpenAccess returns an AccessPod for the bot. Reuses the running bot pod
// when possible; otherwise spawns a short-lived alpine pod with the same PVC
// + subpath mounted.
func OpenAccess(ctx context.Context, botID string) (*AccessPod, error) {
	if podName, err := GetPodName(ctx, botID); err == nil {
		return &AccessPod{
			Namespace: GetNamespace(),
			Name:      podName,
			Container: "openclaw",
			MountPath: "/home/node/.openclaw",
			BotID:     botID,
		}, nil
	}
	return spawnAccessPod(ctx, botID)
}

func spawnAccessPod(ctx context.Context, botID string) (*AccessPod, error) {
	client := GetClient()
	namespace := GetNamespace()
	pvcName := viper.GetString("storage.pvc_name")
	if pvcName == "" {
		pvcName = "openclaw-shared-data"
	}
	imagePullSecret := viper.GetString("openclaw.image_pull_secret")

	suffix, err := randHex(4)
	if err != nil {
		return nil, err
	}
	podName := fmt.Sprintf("clawhost-pvc-%s-%s", getShortID(botID), suffix)

	uid := int64(1000)
	gid := int64(1000)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":    "clawhost-pvc-access",
				"bot-id": botID,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:  &uid,
				RunAsGroup: &gid,
				FSGroup:    &gid,
			},
			ImagePullSecrets: func() []corev1.LocalObjectReference {
				if imagePullSecret != "" {
					return []corev1.LocalObjectReference{{Name: imagePullSecret}}
				}
				return nil
			}(),
			Containers: []corev1.Container{
				{
					Name:    "shell",
					Image:   "alpine:3.19",
					Command: []string{"sleep", "3600"},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "data", MountPath: "/pvc", SubPath: botID},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("create access pod: %w", err)
	}

	cleanup := func() {
		gp := int64(0)
		bg := metav1.DeletePropagationBackground
		_ = client.CoreV1().Pods(namespace).Delete(context.Background(), podName, metav1.DeleteOptions{
			GracePeriodSeconds: &gp,
			PropagationPolicy:  &bg,
		})
	}

	if err := waitAccessPodReady(ctx, namespace, podName, 90*time.Second); err != nil {
		cleanup()
		return nil, err
	}

	return &AccessPod{
		Namespace: namespace,
		Name:      podName,
		Container: "shell",
		MountPath: "/pvc",
		BotID:     botID,
		cleanup:   cleanup,
	}, nil
}

func waitAccessPodReady(ctx context.Context, namespace, podName string, timeout time.Duration) error {
	client := GetClient()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err == nil && pod.Status.Phase == corev1.PodRunning {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("timeout waiting for access pod %s", podName)
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
