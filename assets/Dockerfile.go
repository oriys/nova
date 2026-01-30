FROM ubuntu:24.04

ARG GO_VERSION=1.22.0

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz | tar -C /usr/local -xzf - \
    && ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create init script
RUN echo '#!/bin/sh\n\
mount -t proc proc /proc\n\
mount -t sysfs sysfs /sys\n\
mount -t devtmpfs devtmpfs /dev\n\
ip link set lo up\n\
ip link set eth0 up 2>/dev/null || true\n\
exec /usr/local/bin/nova-agent' > /init && chmod +x /init

COPY bin/nova-agent /usr/local/bin/nova-agent
RUN chmod +x /usr/local/bin/nova-agent 2>/dev/null || true

CMD ["/init"]
