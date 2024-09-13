package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/darox/k8s-iperf/pkg/iperf"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var rootCmd = &cobra.Command{
	Use:   "k8s-iperf",
	Short: "Run iperf tests on Kubernetes clusters",
}

var runCmd = &cobra.Command{
	Use:   "run [flags] -- [iperf args]",
	Short: "Run an iperf test",
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace, _ := cmd.Flags().GetString("k8s-namespace")
		image, _ := cmd.Flags().GetString("k8s-image")
		serverNode, _ := cmd.Flags().GetString("k8s-server-node")
		clientNode, _ := cmd.Flags().GetString("k8s-client-node")
		domain, _ := cmd.Flags().GetString("k8s-domain")
		cleanup, _ := cmd.Flags().GetBool("k8s-cleanup")

		clientset, err := getClientset()
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes client: %w", err)
		}

		// Extract iperf args
		iperfArgs := []string{}
		if cmd.ArgsLenAtDash() != -1 {
			iperfArgs = args[cmd.ArgsLenAtDash():]
		}

		config := iperf.TestConfig{
			Client:     clientset,
			Namespace:  namespace,
			Image:      image,
			IperfArgs:  iperfArgs,
			ServerNode: serverNode,
			ClientNode: clientNode,
			Domain:     domain,
			Cleanup:    cleanup,
		}

		return iperf.RunTest(config)
	},
}

func getClientset() (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	// Check if running inside a Kubernetes cluster
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		// Use in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		// Use kubeconfig
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func init() {
	runCmd.Flags().StringP("k8s-namespace", "", "default", "Kubernetes namespace to run the test in")
	runCmd.Flags().StringP("k8s-image", "", "dariomader/iperf3:latest", "Docker image to use for the test")
	runCmd.Flags().StringP("k8s-server-node", "", "", "Server node to use for the test")
	runCmd.Flags().StringP("k8s-client-node", "", "", "Client node to use for the test")
	runCmd.Flags().StringP("k8s-domain", "", "cluster.local", "Kubernetes domain to use for the test")
	runCmd.Flags().BoolP("k8s-cleanup", "", true, "Cleanup resources after the test")
	rootCmd.AddCommand(runCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
