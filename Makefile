.PHONY: build clean install setup-dirs

BINARY_DIR := bin
NOVA_BIN := $(BINARY_DIR)/nova
AGENT_BIN := $(BINARY_DIR)/nova-agent

build: $(NOVA_BIN) $(AGENT_BIN)

$(NOVA_BIN): cmd/nova/main.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -o $@ ./cmd/nova

$(AGENT_BIN): cmd/agent/main.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/agent

clean:
	rm -rf $(BINARY_DIR)

install: build
	cp $(NOVA_BIN) /usr/local/bin/nova

setup-dirs:
	sudo mkdir -p /opt/nova/{kernel,rootfs}
	sudo mkdir -p /tmp/nova/{sockets,vsock,logs}
	sudo chown -R $(USER):$(USER) /tmp/nova

# Download firecracker binary (Linux only)
setup-firecracker:
	@echo "Downloading Firecracker..."
	curl -fsSL -o /tmp/firecracker.tgz \
		https://github.com/firecracker-microvm/firecracker/releases/download/v1.7.0/firecracker-v1.7.0-x86_64.tgz
	tar -xzf /tmp/firecracker.tgz -C /tmp
	sudo mv /tmp/release-v1.7.0-x86_64/firecracker-v1.7.0-x86_64 /usr/bin/firecracker
	sudo chmod +x /usr/bin/firecracker
	rm -rf /tmp/firecracker.tgz /tmp/release-v1.7.0-x86_64

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
