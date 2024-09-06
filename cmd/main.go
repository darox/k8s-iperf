package main

import (
	"fmt"
	"os"

	"github.com/darox/k8s-iperf/pkg/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
