.PHONY: build build-linux clean install deploy setup-dirs setup-firecracker setup-kernel setup-rootfs setup-all

BINARY_DIR := bin
NOVA_BIN := $(BINARY_DIR)/nova
NOVA_LINUX := $(BINARY_DIR)/nova-linux
AGENT_BIN := $(BINARY_DIR)/nova-agent
ASSETS_DIR := assets
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

install: build
	sudo cp $(NOVA_BIN) /usr/local/bin/nova

# Deploy to Linux server (from macOS)
# Usage: make deploy SERVER=user@your-server
deploy: build-linux
	@echo "Deploying to $(SERVER)..."
	./scripts/deploy.sh $(SERVER)

# Setup runtime directories
setup-dirs:
	sudo mkdir -p /opt/nova/{kernel,rootfs}
	sudo mkdir -p /tmp/nova/{sockets,vsock,logs}
	sudo chown -R $(USER):$(USER) /tmp/nova /opt/nova

# Download Firecracker binary (Linux x86_64 only)
setup-firecracker:
	@echo "Downloading Firecracker v1.7.0..."
	curl -fsSL -o /tmp/firecracker.tgz \
		https://github.com/firecracker-microvm/firecracker/releases/download/v1.7.0/firecracker-v1.7.0-x86_64.tgz
	tar -xzf /tmp/firecracker.tgz -C /tmp
	sudo mv /tmp/release-v1.7.0-x86_64/firecracker-v1.7.0-x86_64 /usr/bin/firecracker
	sudo chmod +x /usr/bin/firecracker
	rm -rf /tmp/firecracker.tgz /tmp/release-v1.7.0-x86_64
	@echo "Firecracker installed: $$(firecracker --version)"

# Download Firecracker kernel
setup-kernel:
	@echo "Downloading Firecracker kernel (Linux 5.10)..."
	mkdir -p $(ASSETS_DIR)/kernel
	curl -fsSL -o $(ASSETS_DIR)/kernel/vmlinux \
		https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.11/x86_64/vmlinux-5.10.225
	sudo cp $(ASSETS_DIR)/kernel/vmlinux /opt/nova/kernel/
	@echo "Kernel ready: /opt/nova/kernel/vmlinux"

# Build rootfs images (requires Linux with root)
setup-rootfs: $(AGENT_BIN)
	@echo "Building rootfs images..."
	chmod +x $(ASSETS_DIR)/setup.sh
	cd $(ASSETS_DIR) && sudo ./setup.sh
	sudo cp $(ASSETS_DIR)/rootfs/*.ext4 /opt/nova/rootfs/
	@echo "Rootfs images ready in /opt/nova/rootfs/"

# Full setup (Linux only)
setup-all: setup-dirs setup-firecracker setup-kernel setup-rootfs
	@echo "Nova setup complete!"
	@echo "  Firecracker: $$(which firecracker)"
	@echo "  Kernel: /opt/nova/kernel/vmlinux"
	@echo "  Rootfs: /opt/nova/rootfs/"

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
