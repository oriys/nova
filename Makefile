.DEFAULT_GOAL := help

# Variables

BINARY_DIR   := bin
NOVA_BIN     := $(BINARY_DIR)/nova
NOVA_LINUX   := $(BINARY_DIR)/nova-linux
AGENT_BIN    := $(BINARY_DIR)/nova-agent
SERVER       ?= user@server
PREFIX       ?= nova-runtime

# ─── Backend ──────────────────────────────────────────────────────────────────

.PHONY: build build-linux agent

build: $(NOVA_BIN) $(AGENT_BIN)  ## Build nova (native) + agent (linux/amd64)

build-linux: $(NOVA_LINUX) $(AGENT_BIN)  ## Cross-compile nova + agent for linux/amd64

agent: $(AGENT_BIN)  ## Build only the guest agent (linux/amd64)

$(NOVA_BIN): cmd/nova/main.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -o $@ ./cmd/nova

$(NOVA_LINUX): cmd/nova/main.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/nova

$(AGENT_BIN): cmd/agent/main.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/agent

# ─── Frontend ─────────────────────────────────────────────────────────────────

.PHONY: frontend frontend-dev

frontend:  ## Build Lumen frontend (npm install + build)
	cd lumen && npm install && npm run build

frontend-dev:  ## Start Lumen dev server on localhost:3000
	cd lumen && npm run dev

# ─── Docker Images ────────────────────────────────────────────────────────────

.PHONY: docker-backend docker-frontend docker-runtimes

docker-backend:  ## Build Nova backend Docker image
	docker build -t nova -f Dockerfile .

docker-frontend:  ## Build Lumen frontend Docker image
	docker build -t lumen -f lumen/Dockerfile ./lumen

docker-runtimes: $(AGENT_BIN)  ## Build all runtime Docker images
	./docker/runtimes/build.sh $(PREFIX)

docker-runtime-%: $(AGENT_BIN)  ## Build a single runtime image (e.g. make docker-runtime-python)
	docker build -f docker/runtimes/Dockerfile.$* -t $(PREFIX)-$* .

# ─── VM Rootfs ────────────────────────────────────────────────────────────────

.PHONY: rootfs download-assets

rootfs: download-assets  ## Build rootfs images using Docker
	docker build -f Dockerfile.rootfs -t nova-rootfs-builder .
	@mkdir -p assets/rootfs
	docker run --rm \
		-v $(PWD)/assets/rootfs:/opt/nova/rootfs \
		nova-rootfs-builder

download-assets:  ## Download large assets (Firecracker binary, kernel, etc.)
	./scripts/download_assets.sh

# ─── All ──────────────────────────────────────────────────────────────────────

.PHONY: all

all: build frontend docker-backend docker-frontend docker-runtimes  ## Build everything (backend + frontend + Docker images)

# ─── Dev Environment ──────────────────────────────────────────────────────────

.PHONY: dev seed

dev: download-assets  ## Start full stack via docker compose (Postgres + Nova + Lumen)
	docker-compose up --build

seed:  ## Seed sample functions via scripts/seed-functions.sh
	./scripts/seed-functions.sh

# ─── Deploy ───────────────────────────────────────────────────────────────────

.PHONY: deploy

deploy: build-linux  ## Cross-compile + deploy to server (SERVER=root@host)
	./scripts/deploy.sh $(SERVER)

# ─── Demo ─────────────────────────────────────────────────────────────────────

.PHONY: demo-register demo-list demo-get demo-invoke demo-delete

demo-register:  ## Register a sample hello-python function
	./$(NOVA_BIN) register hello-python \
		--runtime python \
		--handler main.handler \
		--code examples/hello.py

demo-list:  ## List all registered functions
	./$(NOVA_BIN) list

demo-get:  ## Get hello-python function details
	./$(NOVA_BIN) get hello-python

demo-invoke:  ## Invoke hello-python with sample payload
	./$(NOVA_BIN) invoke hello-python --payload '{"name": "World"}'

demo-delete:  ## Delete hello-python function
	./$(NOVA_BIN) delete hello-python

# ─── Clean ────────────────────────────────────────────────────────────────────

.PHONY: clean clean-all

clean:  ## Remove bin/ directory
	rm -rf $(BINARY_DIR)

clean-all: clean  ## Remove bin/ + assets/ + lumen build artifacts
	rm -rf assets/
	rm -rf lumen/.next lumen/node_modules

# ─── Help ─────────────────────────────────────────────────────────────────────

.PHONY: help

help:  ## Show targets (interactive with fzf, static otherwise)
	@if command -v fzf >/dev/null 2>&1; then \
		target=$$(awk 'BEGIN {FS = ":.*##"} \
			/^# ─── / { sub(/^# ─── /, ""); sub(/ ───.*/, ""); group=$$0; next } \
			/^[a-zA-Z_%-]+:.*##/ { printf "[%s]  %-20s %s\n", group, $$1, $$2 }' $(MAKEFILE_LIST) \
			| fzf --ansi --prompt="make ❯ " --header="Select a target to run" --reverse \
			| awk '{print $$2}'); \
		if [ -n "$$target" ]; then \
			echo ""; \
			$(MAKE) --no-print-directory $$target; \
		fi; \
	else \
		echo "Usage: make [target]"; \
		echo ""; \
		awk 'BEGIN {FS = ":.*##"} \
			/^# ─── / { sub(/^# ─── /, ""); sub(/ ───.*/, ""); printf "\n\033[1m%s\033[0m\n", $$0; next } \
			/^[a-zA-Z_%-]+:.*##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST); \
		echo ""; \
	fi
