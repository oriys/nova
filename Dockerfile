FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.cn,direct go mod download

# Copy source code with dependency order in mind
COPY api/ api/
COPY internal/ internal/
COPY cmd/ cmd/

# Build with optimizations
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o nova ./cmd/nova

FROM alpine:3.19
RUN apk add --no-cache ca-certificates docker-cli
WORKDIR /app

# Copy binary
COPY --from=builder /app/nova /usr/local/bin/nova

# Copy configuration files
COPY configs/ /app/configs/

EXPOSE 9000
# Ensure it uses the config file in the container
CMD ["nova", "daemon", "--config", "configs/nova.json", "--http", ":9000"]
