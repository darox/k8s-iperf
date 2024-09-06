package iperf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// At the package level, add this static error:
var errIperf3ClientPodFailed = errors.New("iperf3 client pod failed")

// TestConfig holds the configuration for the iperf3 test
type TestConfig struct {
	Client     *kubernetes.Clientset
	Namespace  string
	Image      string
	IperfArgs  []string
	ServerNode string
	ClientNode string
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
	// Deploy iperf3 server
	if err := deployIperf3Server(config); err != nil {
		return fmt.Errorf("failed to deploy iperf3 server: %w", err)
	}
	fmt.Println("iperf3 server deployed successfully")

	// Create service for iperf3 server
	if err := createIperf3Service(config.Client, config.Namespace); err != nil {
		return fmt.Errorf("failed to create iperf3 service: %w", err)
	}
	fmt.Println("iperf3 service created successfully")

	// Wait for server to be ready
	if err := waitForDeploymentReady(config.Client, config.Namespace, "iperf3-server", 60*time.Second); err != nil {
		return fmt.Errorf("iperf3 server failed to become ready: %w", err)
	}

	// Run iperf3 client
	fmt.Println("Running iperf3 test................")
	err := runIperf3Client(ctx, config)
	if err != nil {
		return fmt.Errorf("iperf3 client test failed: %w", err)
	}

	// Cleanup resources
	if err := cleanup(config.Client, config.Namespace); err != nil {
		return fmt.Errorf("failed to cleanup resources: %w", err)
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
	args := []string{"-c", "iperf3-server"}
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

	fmt.Println(string(logs))

	if podPhase == corev1.PodFailed {
		return fmt.Errorf("iperf3 client execution: %w", errIperf3ClientPodFailed)
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
