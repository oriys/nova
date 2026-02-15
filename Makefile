.DEFAULT_GOAL := help

# Variables

BINARY_DIR   := bin
NOVA_BIN     := $(BINARY_DIR)/nova
NOVA_LINUX   := $(BINARY_DIR)/nova-linux
COMET_BIN    := $(BINARY_DIR)/comet
COMET_LINUX  := $(BINARY_DIR)/comet-linux
ZENITH_BIN   := $(BINARY_DIR)/zenith
ZENITH_LINUX := $(BINARY_DIR)/zenith-linux
CORONA_BIN   := $(BINARY_DIR)/corona
CORONA_LINUX := $(BINARY_DIR)/corona-linux
NEBULA_BIN   := $(BINARY_DIR)/nebula
NEBULA_LINUX := $(BINARY_DIR)/nebula-linux
AURORA_BIN   := $(BINARY_DIR)/aurora
AURORA_LINUX := $(BINARY_DIR)/aurora-linux
AGENT_BIN    := $(BINARY_DIR)/nova-agent
ATLAS_BIN    := $(BINARY_DIR)/atlas
ATLAS_LINUX  := $(BINARY_DIR)/atlas-linux
SERVER       ?= user@server
PREFIX       ?= nova-runtime

# ─── Backend ──────────────────────────────────────────────────────────────────

.PHONY: build build-linux agent comet comet-linux zenith zenith-linux corona corona-linux nebula nebula-linux aurora aurora-linux proto

proto:  ## Generate gRPC/protobuf code via buf (nova.proto)
	cd api/proto && buf generate --path nova.proto

build: $(NOVA_BIN) $(COMET_BIN) $(ZENITH_BIN) $(CORONA_BIN) $(NEBULA_BIN) $(AURORA_BIN) $(AGENT_BIN)  ## Build all services (native) + agent (linux/amd64)

build-linux: $(NOVA_LINUX) $(COMET_LINUX) $(ZENITH_LINUX) $(CORONA_LINUX) $(NEBULA_LINUX) $(AURORA_LINUX) $(AGENT_BIN)  ## Cross-compile all services + agent for linux/amd64

agent: $(AGENT_BIN)  ## Build only the guest agent (linux/amd64)
comet: $(COMET_BIN)  ## Build Comet data plane (native)
comet-linux: $(COMET_LINUX)  ## Cross-compile Comet for linux/amd64
zenith: $(ZENITH_BIN)  ## Build Zenith gateway (native)
zenith-linux: $(ZENITH_LINUX)  ## Cross-compile Zenith for linux/amd64
corona: $(CORONA_BIN)  ## Build Corona scheduler/placement plane (native)
corona-linux: $(CORONA_LINUX)  ## Cross-compile Corona for linux/amd64
nebula: $(NEBULA_BIN)  ## Build Nebula event ingestion plane (native)
nebula-linux: $(NEBULA_LINUX)  ## Cross-compile Nebula for linux/amd64
aurora: $(AURORA_BIN)  ## Build Aurora observability plane (native)
aurora-linux: $(AURORA_LINUX)  ## Cross-compile Aurora for linux/amd64

$(NOVA_BIN): cmd/nova/main.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -o $@ ./cmd/nova

$(NOVA_LINUX): cmd/nova/main.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/nova

$(COMET_BIN): cmd/comet/main.go cmd/comet/daemon.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -o $@ ./cmd/comet

$(COMET_LINUX): cmd/comet/main.go cmd/comet/daemon.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/comet

$(ZENITH_BIN): cmd/zenith/main.go cmd/zenith/serve.go internal/**/*.go api/proto/novapb/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -o $@ ./cmd/zenith

$(ZENITH_LINUX): cmd/zenith/main.go cmd/zenith/serve.go internal/**/*.go api/proto/novapb/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/zenith

$(CORONA_BIN): cmd/corona/main.go cmd/corona/daemon.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -o $@ ./cmd/corona

$(CORONA_LINUX): cmd/corona/main.go cmd/corona/daemon.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/corona

$(NEBULA_BIN): cmd/nebula/main.go cmd/nebula/daemon.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -o $@ ./cmd/nebula

$(NEBULA_LINUX): cmd/nebula/main.go cmd/nebula/daemon.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/nebula

$(AURORA_BIN): cmd/aurora/main.go cmd/aurora/daemon.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -o $@ ./cmd/aurora

$(AURORA_LINUX): cmd/aurora/main.go cmd/aurora/daemon.go internal/**/*.go
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/aurora

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

# ─── Orbit CLI ───────────────────────────────────────────────────────────────

.PHONY: orbit orbit-release orbit-clean

orbit:  ## Build Orbit CLI (debug)
	cd orbit && cargo build

orbit-release:  ## Build Orbit CLI (release)
	cd orbit && cargo build --release

orbit-clean:  ## Clean Orbit build artifacts
	cd orbit && cargo clean

# ─── Atlas MCP Server ────────────────────────────────────────────────────────

.PHONY: atlas atlas-linux atlas-clean

atlas: $(ATLAS_BIN)  ## Build Atlas MCP server

atlas-linux: $(ATLAS_LINUX)  ## Cross-compile Atlas for linux/amd64

atlas-clean:  ## Clean Atlas build artifacts
	rm -f $(ATLAS_BIN) $(ATLAS_LINUX)

$(ATLAS_BIN): atlas/*.go
	@mkdir -p $(BINARY_DIR)
	cd atlas && CGO_ENABLED=0 go build -o ../$(ATLAS_BIN) .

$(ATLAS_LINUX): atlas/*.go
	@mkdir -p $(BINARY_DIR)
	cd atlas && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../$(ATLAS_LINUX) .

# ─── All ──────────────────────────────────────────────────────────────────────

.PHONY: all

all: build orbit atlas frontend docker-backend docker-frontend docker-runtimes  ## Build everything (backend + frontend + CLI + MCP + Docker images)

# ─── Testing ──────────────────────────────────────────────────────────────────

.PHONY: test test-unit test-integration env-up env-down

test: test-unit  ## Run all tests (unit + available integration)

test-unit:  ## Run unit tests (no external dependencies)
	go test -short -count=1 ./internal/...

test-integration:  ## Run integration tests (requires env-up)
	NOVA_PG_DSN=postgres://nova:nova@localhost:$${NOVA_TEST_PG_PORT:-5433}/nova?sslmode=disable \
	go test -count=1 -run Integration ./internal/...

env-up:  ## Start test dependencies (Postgres)
	docker compose -f docker-compose.test.yml up -d --wait

env-down:  ## Stop test dependencies
	docker compose -f docker-compose.test.yml down -v

# ─── Dev Environment ──────────────────────────────────────────────────────────

.PHONY: dev seed

dev: download-assets  ## Start full stack via docker compose (Postgres + Nova + Comet + Corona + Nebula + Aurora + Zenith + Lumen)
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

clean-all: clean orbit-clean atlas-clean  ## Remove bin/ + assets/ + lumen + orbit + atlas build artifacts
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
