package iperf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"encoding/json"

	"github.com/fatih/color"
)

// At the package level, add this static error:
var errIperf3ClientPodFailed = errors.New("iperf3 client pod failed")

// Add static error variables
var (
	errEmptyLogs             = errors.New("iperf3 client returned empty logs")
	errMissingStartField     = errors.New("missing or invalid 'start' field in JSON data")
	errMissingEndField       = errors.New("missing or invalid 'end' field in JSON data")
	errMissingConnectedField = errors.New("missing or invalid 'connected' field in JSON data")
	errInvalidConnectedData  = errors.New("invalid 'connected' data structure in JSON data")
)

// TestConfig holds the configuration for the iperf3 test
type TestConfig struct {
	Client     *kubernetes.Clientset
	Namespace  string
	Domain     string
	Image      string
	IperfArgs  []string
	ServerNode string
	ClientNode string
	Cleanup    bool
}

func RunTest(config TestConfig) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	// Notify the channel for SIGINT and SIGTERM
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		errChan <- runTestInternal(ctx, config)
	}()

	select {
	case err := <-errChan:
		return err
	case <-sigChan:
		fmt.Println("Received interrupt signal, cleaning up resources...")
		cancel() // Cancel the context to stop ongoing operations
		return cleanup(config.Client, config.Namespace)
	case <-ctx.Done():
		fmt.Println("Test cancelled, cleaning up resources...")
		return cleanup(config.Client, config.Namespace)
	}
}

func runTestInternal(ctx context.Context, config TestConfig) error {
	// Defer cleanup if Cleanup is set to true
	if config.Cleanup {
		defer func() {
			if err := cleanup(config.Client, config.Namespace); err != nil {
				fmt.Printf("Failed to cleanup resources: %v\n", err)
			}
		}()
	}

	// Deploy iperf3 server
	if err := deployIperf3Server(config); err != nil {
		color.Red("✘ Failed to deploy iperf3 server: %v", err)
		return fmt.Errorf("failed to deploy iperf3 server: %w", err)
	}
	color.Green("✔ iperf3 server deployed successfully")

	// Create service for iperf3 server
	if err := createIperf3Service(config.Client, config.Namespace); err != nil {
		color.Red("✘ Failed to create iperf3 service: %v", err)
		return fmt.Errorf("failed to create iperf3 service: %w", err)
	}
	color.Green("✔ iperf3 service created successfully")

	// Wait for server to be ready
	if err := waitForDeploymentReady(config.Client, config.Namespace, "iperf3-server", 60*time.Second); err != nil {
		color.Red("✘ iperf3 server failed to become ready: %v", err)
		return fmt.Errorf("iperf3 server failed to become ready: %w", err)
	}

	// Add this: Wait for the iperf3 server pod to be ready
	if err := waitForPodReady(ctx, config.Client, config.Namespace, "app=iperf3-server", 60*time.Second); err != nil {
		color.Red("✘ iperf3 server pod failed to become ready: %v", err)
		return fmt.Errorf("iperf3 server pod failed to become ready: %w", err)
	}

	color.Green("✔ iperf3 server is up and ready")

	err := runIperf3Client(ctx, config)
	if err != nil {
		color.Red("✘ iperf3 client test failed: %v", err)
		return fmt.Errorf("iperf3 client test failed: %w", err)
	}

	return nil
}

func deployIperf3Server(config TestConfig) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "iperf3-server",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "iperf3-server",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "iperf3-server",
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  int64Ptr(10000),
						RunAsGroup: int64Ptr(10000),
						FSGroup:    int64Ptr(10000),
					},
					Containers: []corev1.Container{
						{
							Name:  "iperf3-server",
							Image: config.Image,
							Args:  []string{"-s"},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 5201,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: boolPtr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								ReadOnlyRootFilesystem: boolPtr(false),
							},
							ImagePullPolicy: corev1.PullAlways,
							StartupProbe: &corev1.Probe{
								Handler: corev1.Handler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(5201),
									},
								},
								InitialDelaySeconds: 5,
								FailureThreshold:    1,
							},
						},
					},
				},
			},
		},
	}

	// Conditionally add NodeSelector if ServerNode is set
	if config.ServerNode != "" {
		deployment.Spec.Template.Spec.NodeSelector = map[string]string{
			"kubernetes.io/hostname": config.ServerNode,
		}
	}

	_, err := config.Client.AppsV1().Deployments(config.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
	return err
}

func createIperf3Service(client *kubernetes.Clientset, namespace string) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "iperf3-server",
			Labels: map[string]string{
				"app": "iperf3-server",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "iperf3-server",
			},
			Ports: []corev1.ServicePort{
				{
					Port:       5201,
					TargetPort: intstr.FromInt(5201),
				},
			},
		},
	}

	_, err := client.CoreV1().Services(namespace).Create(context.TODO(), service, metav1.CreateOptions{})
	return err
}

func runIperf3Client(ctx context.Context, config TestConfig) error {
	serverFQDN := fmt.Sprintf("iperf3-server.%s.svc.%s", config.Namespace, config.Domain)
	args := []string{"-c", serverFQDN, "-J"}
	args = append(args, config.IperfArgs...)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "iperf3-client",
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:  int64Ptr(10000),
				RunAsGroup: int64Ptr(10000),
				FSGroup:    int64Ptr(10000),
			},
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "iperf3-client",
					Image: config.Image,
					Args:  args,
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: boolPtr(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						ReadOnlyRootFilesystem: boolPtr(false),
					},
					ImagePullPolicy: corev1.PullAlways,
				},
			},
		},
	}

	// Conditionally add NodeSelector if ClientNode is set
	if config.ClientNode != "" {
		pod.Spec.NodeSelector = map[string]string{
			"kubernetes.io/hostname": config.ClientNode,
		}
	}

	_, err := config.Client.CoreV1().Pods(config.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	color.Green("✔ iperf3 client pod created successfully")

	color.Green("► Starting iperf3 test")

	// Wait for the client pod to complete
	var podPhase corev1.PodPhase
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			pod, err := config.Client.CoreV1().Pods(config.Namespace).Get(ctx, "iperf3-client", metav1.GetOptions{})
			if err != nil {
				return err
			}

			if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				podPhase = pod.Status.Phase
				goto PodCompleted
			}

			time.Sleep(1 * time.Second)
		}
	}

PodCompleted:
	// Get and print the logs
	logs, err := config.Client.CoreV1().Pods(config.Namespace).GetLogs("iperf3-client", &corev1.PodLogOptions{}).Do(ctx).Raw()
	if err != nil {
		return err
	}

	// Check if logs are empty
	if len(logs) == 0 {
		color.Red("✘ iperf3 client returned empty logs")
		return errEmptyLogs
	}

	// Parse and print the formatted summary
	if err := printIperfSummary(logs); err != nil {
		color.Red("✘ Failed to parse iperf output: %v", err)
		return fmt.Errorf("failed to parse iperf output: %w", err)
	}

	if podPhase == corev1.PodFailed {
		return errIperf3ClientPodFailed
	}

	return nil
}

func printIperfSummary(jsonData []byte) error {
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return fmt.Errorf("failed to unmarshal JSON data: %w", err)
	}

	// Check if the required fields exist
	start, ok := result["start"].(map[string]interface{})
	if !ok {
		return errMissingStartField
	}

	end, ok := result["end"].(map[string]interface{})
	if !ok {
		return errMissingEndField
	}

	connected, ok := start["connected"].([]interface{})
	if !ok || len(connected) == 0 {
		return errMissingConnectedField
	}

	connectedMap, ok := connected[0].(map[string]interface{})
	if !ok {
		return errInvalidConnectedData
	}

	// Extract relevant information
	testStart := start["test_start"].(map[string]interface{})
	sumSent := end["sum_sent"].(map[string]interface{})
	sumReceived := end["sum_received"].(map[string]interface{})

	// Print summary
	fmt.Println() // Add a line break here
	summaryTitle := "iPerf3 Test Summary"
	color.Cyan(summaryTitle)

	cpuUtil := end["cpu_utilization_percent"].(map[string]interface{})

	// Prepare all lines
	lines := []string{
		fmt.Sprintf("Connection Details:"),
		fmt.Sprintf("  Local:  %s:%d", connectedMap["local_host"], int(connectedMap["local_port"].(float64))),
		fmt.Sprintf("  Remote: %s:%d", connectedMap["remote_host"], int(connectedMap["remote_port"].(float64))),
		fmt.Sprintf(""),
		fmt.Sprintf("Test Configuration:"),
		fmt.Sprintf("  Protocol: %s", testStart["protocol"]),
		fmt.Sprintf("  Duration: %.2f seconds", sumSent["seconds"]),
		fmt.Sprintf("  Parallel Streams: %d", int(testStart["num_streams"].(float64))),
		fmt.Sprintf(""),
		fmt.Sprintf("Results:"),
		fmt.Sprintf("  Sent:     %.2f Mbits/sec", sumSent["bits_per_second"].(float64)/1e6),
		fmt.Sprintf("  Received: %.2f Mbits/sec", sumReceived["bits_per_second"].(float64)/1e6),
	}

	if retransmits, ok := sumSent["retransmits"]; ok {
		lines = append(lines, fmt.Sprintf("  Retransmits: %d", int(retransmits.(float64))))
	}

	lines = append(lines,
		fmt.Sprintf(""),
		fmt.Sprintf("CPU Utilization:"),
		fmt.Sprintf("  Local:  %.2f%%", cpuUtil["host_total"].(float64)),
		fmt.Sprintf("  Remote: %.2f%%", cpuUtil["remote_total"].(float64)),
	)

	// Calculate the length of the longest line
	maxLength := len(summaryTitle)
	for _, line := range lines {
		if len(line) > maxLength {
			maxLength = len(line)
		}
	}

	// Print dashes
	color.Cyan(strings.Repeat("-", maxLength))

	// Print all lines
	for _, line := range lines {
		if strings.HasPrefix(line, "Connection Details:") ||
			strings.HasPrefix(line, "Test Configuration:") ||
			strings.HasPrefix(line, "Results:") ||
			strings.HasPrefix(line, "CPU Utilization:") {
			color.Yellow(line)
		} else {
			fmt.Println(line)
		}
	}

	return nil
}

func cleanup(client *kubernetes.Clientset, namespace string) error {
	if err := client.AppsV1().Deployments(namespace).Delete(context.TODO(), "iperf3-server", metav1.DeleteOptions{}); err != nil {
		return err
	}

	if err := client.CoreV1().Services(namespace).Delete(context.TODO(), "iperf3-server", metav1.DeleteOptions{}); err != nil {
		return err
	}

	return client.CoreV1().Pods(namespace).Delete(context.TODO(), "iperf3-client", metav1.DeleteOptions{})
}

func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func waitForDeploymentReady(client *kubernetes.Clientset, namespace, deploymentName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, func() (bool, error) {
		deployment, err := client.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return deployment.Status.ReadyReplicas == *deployment.Spec.Replicas, nil
	})
}

// Add this new function
func waitForPodReady(ctx context.Context, client *kubernetes.Clientset, namespace, labelSelector string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, func() (bool, error) {
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return false, err
		}

		if len(pods.Items) == 0 {
			return false, nil
		}

		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true, nil
					}
				}
			}
		}

		return false, nil
	})
}
