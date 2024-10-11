package cli

import (
	"fmt"

	"github.com/darox/k8s-iperf/pkg/k8s"

	"github.com/darox/k8s-iperf/pkg/iperf"

	"github.com/spf13/cobra"
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
		serverContext, _ := cmd.Flags().GetString("k8s-server-context")
		clientContext, _ := cmd.Flags().GetString("k8s-client-context")
		multiCluster, _ := cmd.Flags().GetBool("k8s-multi-cluster")
		serviceAnnotation, _ := cmd.Flags().GetString("k8s-service-annotation")
		serverClientSet, err := k8s.NewClient(serverContext)
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes client: %w", err)
		}

		clientClientSet, err := k8s.NewClient(clientContext)
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes client: %w", err)
		}

		// Extract iperf args
		iperfArgs := []string{}
		if cmd.ArgsLenAtDash() != -1 {
			iperfArgs = args[cmd.ArgsLenAtDash():]
		}

		config := iperf.TestConfig{
			ClientClientSet:   clientClientSet,
			ServerClientSet:   serverClientSet,
			Namespace:         namespace,
			Image:             image,
			IperfArgs:         iperfArgs,
			ServerNode:        serverNode,
			ClientNode:        clientNode,
			Domain:            domain,
			Cleanup:           cleanup,
			MultiCluster:      multiCluster,
			ServiceAnnotation: serviceAnnotation,
		}

		return iperf.RunTest(config)
	},
}

func init() {
	runCmd.Flags().StringP("k8s-namespace", "", "default", "Kubernetes namespace to run the test in")
	runCmd.Flags().StringP("k8s-image", "", "dariomader/iperf3:latest", "Docker image to use for the test")
	runCmd.Flags().StringP("k8s-server-node", "", "", "Server node to use for the test")
	runCmd.Flags().StringP("k8s-client-node", "", "", "Client node to use for the test")
	runCmd.Flags().StringP("k8s-domain", "", "cluster.local", "Kubernetes domain to use for the test")
	runCmd.Flags().BoolP("k8s-cleanup", "", true, "Cleanup resources after the test")
	runCmd.Flags().StringP("k8s-server-context", "", "", "Kubernetes server context to use for the test")
	runCmd.Flags().StringP("k8s-client-context", "", "", "Kubernetes client context to use for the test")
	runCmd.Flags().BoolP("k8s-multi-cluster", "", false, "Run the test in multi-cluster mode")
	runCmd.Flags().StringP("k8s-service-annotation", "", "", "Service annotation to use for the test (signature: key1=value1,key2=value2)")
	rootCmd.AddCommand(runCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
