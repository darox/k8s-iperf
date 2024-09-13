# Build stage
FROM alpine:3.18 AS builder

# Update and upgrade all packages, including openssl
RUN apk update && apk upgrade

# Install iperf3 and create a non-root user
RUN apk add --no-cache iperf3 && \
    adduser -D -H -u 10000 -s /sbin/nologin iperf

# Final stage
FROM alpine:3.18

# Update and upgrade all packages, including openssl
RUN apk update && apk upgrade

# Install iperf3 and necessary runtime dependencies
RUN apk add --no-cache iperf3 libgcc libstdc++

# Copy user information
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Set working directory
WORKDIR /tmp

# Drop privileges
USER iperf

# Set the entrypoint to the iperf3 binary
ENTRYPOINT ["/usr/bin/iperf3"]

# Default command (can be overridden)
CMD ["-s"]
