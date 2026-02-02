.PHONY: build build-linux clean deploy

BINARY_DIR := bin
NOVA_BIN := $(BINARY_DIR)/nova
NOVA_LINUX := $(BINARY_DIR)/nova-linux
AGENT_BIN := $(BINARY_DIR)/nova-agent
SERVER ?= user@server

build: $(NOVA_BIN) $(AGENT_BIN)

build-linux: $(NOVA_LINUX) $(AGENT_BIN)

$(NOVA_BIN): cmd/nova/main.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -o $@ ./cmd/nova

$(NOVA_LINUX): cmd/nova/main.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/nova

$(AGENT_BIN): cmd/agent/main.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/agent

clean:
	rm -rf $(BINARY_DIR)

# Deploy to Linux server from macOS
# Usage: make deploy SERVER=root@your-server
deploy: build-linux
	./scripts/deploy.sh $(SERVER)

download-assets:
	./scripts/download_assets.sh

# Build rootfs images using Docker (avoids local dependencies)
rootfs-docker: download-assets
	docker build -f Dockerfile.rootfs -t nova-rootfs-builder .
	@mkdir -p assets/rootfs
	docker run --rm \
		-v $(PWD)/assets/rootfs:/opt/nova/rootfs \
		nova-rootfs-builder

# Run full stack (Postgres + Nova + Dashboard)
# Note: Requires Linux with KVM enabled. On macOS, Nova will start but fail to launch VMs.
dev: download-assets
	docker-compose up --build

# Helper targets for demo
demo-register:
	./$(NOVA_BIN) register hello-python \
		--runtime python \
		--handler main.handler \
		--code examples/hello.py

demo-list:
	./$(NOVA_BIN) list

demo-get:
	./$(NOVA_BIN) get hello-python

demo-invoke:
	./$(NOVA_BIN) invoke hello-python --payload '{"name": "World"}'

demo-delete:
	./$(NOVA_BIN) delete hello-python
