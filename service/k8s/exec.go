package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ListOptions returns list options for querying pods by bot ID
func ListOptions(deploymentName string) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=openclaw,bot-id=%s", extractBotID(deploymentName)),
	}
}

// extractBotID extracts bot ID from deployment name
func extractBotID(deploymentName string) string {
	// deploymentName format: openclaw-{botID}
	if len(deploymentName) > 9 {
		return deploymentName[9:]
	}
	return deploymentName
}

// GetPodName returns the name of the running pod for a bot
func GetPodName(ctx context.Context, botID string) (string, error) {
	client := GetClient()
	namespace := GetNamespace()

	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=openclaw,bot-id=%s", botID),
	})
	if err != nil {
		return "", err
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf("no running pod found for bot %s", botID)
}

// WaitForPodReady waits for a pod to be ready and returns its name
func WaitForPodReady(ctx context.Context, botID string, timeoutSeconds int) (string, error) {
	client := GetClient()
	namespace := GetNamespace()

	for i := 0; i < timeoutSeconds; i++ {
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=openclaw,bot-id=%s", botID),
		})
		if err != nil {
			return "", err
		}

		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						return pod.Name, nil
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
			// Wait 1 second before retry
		}
	}

	return "", fmt.Errorf("timeout waiting for pod to be ready")
}

// ExecInPod executes a command in a pod container
func ExecInPod(ctx context.Context, namespace, podName, containerName string, command []string) (string, error) {
	client := GetClient()
	config := GetRestConfig()

	if config == nil {
		return "", fmt.Errorf("rest config not initialized")
	}

	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", fmt.Errorf("exec failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// ExecStreamOptions configures a streaming exec call.
type ExecStreamOptions struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	TTY    bool
}

// ExecStream runs a command in a pod container, streaming stdin/stdout/stderr
// through the provided readers/writers. Use this when the command needs to
// pipe binary data (e.g. tar) or produce live output.
func ExecStream(ctx context.Context, namespace, podName, container string, command []string, opts ExecStreamOptions) error {
	config := GetRestConfig()
	if config == nil {
		return fmt.Errorf("rest config not initialized")
	}
	client := GetClient()

	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     opts.Stdin != nil,
			Stdout:    opts.Stdout != nil,
			Stderr:    opts.Stderr != nil,
			TTY:       opts.TTY,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}
	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  opts.Stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
		Tty:    opts.TTY,
	})
}
