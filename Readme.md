# k8s-iperf

![k8s-iperf screenshot](assets/screenshot.png)

k8s-iperf is a tool to test the network performance between two nodes in a Kubernetes cluster using iperf3.

## Installation

To use k8s-iperf, you need to have access to a Kubernetes cluster and the `kubectl` command-line tool configured.

### Build from source

```
make build
```

or

```
go build -ldflags="-s -w" -o k8s-iperf ./cmd/main.go
```

### Install binary

```
sudo mv k8s-iperf /usr/local/bin/
```

## Usage

The basic command to run an iperf test is:

```
k8s-iperf run
```

Running in multi-cluster mode over Cilium Cluster Mesh:

```
k8s-iperf run --k8s-service-annotation "service.cilium.io/global=true" --k8s-multi-cluster --k8s-client-context minikube --k8s-server-context kind
```

### Flags

- `--k8s-namespace` Specify the Kubernetes namespace to run the test in (default: "default")
- `--k8s-image` Specify the Docker image to use for the test (default: "dariomader/iperf3:latest")
- `--k8s-server-node` Specify the Kubernetes node to run the iperf3 server on
- `--k8s-client-node` Specify the Kubernetes node to run the iperf3 client on
- `--k8s-service-annotation` Specify the service annotation for the iperf3 server (signature: key1=value1,key2=value2)
- `--k8s-client-context` Specify the Kubernetes client context to use for the test
- `--k8s-server-context` Specify the Kubernetes server context to use for the test
- `--k8s-multi-cluster` Run the test in multi-cluster mode

### Iperf Arguments

You can pass additional iperf3 arguments after the `--` separator. These will be forwarded to the iperf3 client.

### Examples

1. Run a basic iperf test in the default namespace:
   ```
   k8s-iperf run
   ```

2. Run a test in a specific namespace:
   ```
   k8s-iperf run --k8s-namespace mynetwork
   ```

3. Use a custom iperf3 image:
   ```
   k8s-iperf run --k8s-image myrepo/custom-iperf3:v1
   ```

4. Run a test between specific nodes:
   ```
   k8s-iperf run --k8s-server-node node1 --k8s-client-node node2
   ```

5. Pass additional iperf3 arguments:
   ```
   k8s-iperf run -- -t 30 -P 4
   ```
   This runs a 30-second test with 4 parallel streams.

6. Combine flags and iperf3 arguments:
   ```
   k8s-iperf run --k8s-namespace mynetwork --k8s-server-node node1 --k8s-client-node node2 -- -t 60 -R
   ```
   This runs a 60-second test in reverse mode in the "mynetwork" namespace between node1 and node2.

## Docker Image

The default iperf3 image is hosted on Docker Hub:

```
docker pull dariomader/iperf3:latest
```

You can also build the image yourself using the Dockerfile in this repository.

As of 06.09.2024 the image has no vulnerabilities according to trivy:

```
trivy image dariomader/iperf3:latest
2024-09-06T13:37:03+02:00       INFO    [vuln] Vulnerability scanning is enabled
2024-09-06T13:37:03+02:00       INFO    [secret] Secret scanning is enabled
2024-09-06T13:37:03+02:00       INFO    [secret] If your scanning is slow, please try '--scanners vuln' to disable secret scanning
2024-09-06T13:37:03+02:00       INFO    [secret] Please see also https://aquasecurity.github.io/trivy/v0.55/docs/scanner/secret#recommendation for faster secret detection
2024-09-06T13:37:03+02:00       INFO    Detected OS     family="alpine" version="3.18.8"
2024-09-06T13:37:03+02:00       INFO    [alpine] Detecting vulnerabilities...   os_version="3.18" repository="3.18" pkg_num=18
2024-09-06T13:37:03+02:00       INFO    Number of language-specific files       num=0

dariomader/iperf3:latest (alpine 3.18.8)

Total: 0 (UNKNOWN: 0, LOW: 0, MEDIUM: 0, HIGH: 0, CRITICAL: 0)
```

The image is also only `17.2MB` in size.

## Future plans
- Add better output formatting



