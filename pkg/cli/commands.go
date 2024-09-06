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
		client, err := k8s.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes client: %w", err)
		}

		// Extract iperf args
		iperfArgs := []string{}
		if cmd.ArgsLenAtDash() != -1 {
			iperfArgs = args[cmd.ArgsLenAtDash():]
		}

		config := iperf.TestConfig{
			Client:     client,
			Namespace:  namespace,
			Image:      image,
			IperfArgs:  iperfArgs,
			ServerNode: serverNode,
			ClientNode: clientNode,
		}

		return iperf.RunTest(config)
	},
}

func init() {
	runCmd.Flags().StringP("k8s-namespace", "", "default", "Kubernetes namespace to run the test in")
	runCmd.Flags().StringP("k8s-image", "", "dariomader/iperf3:latest", "Docker image to use for the test")
	runCmd.Flags().StringP("k8s-server-node", "", "", "Server node to use for the test")
	runCmd.Flags().StringP("k8s-client-node", "", "", "Client node to use for the test")
	rootCmd.AddCommand(runCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
