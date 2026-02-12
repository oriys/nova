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
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/nova ./cmd/nova && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/comet ./cmd/comet && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/zenith ./cmd/zenith && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/corona ./cmd/corona && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/nebula ./cmd/nebula && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/aurora ./cmd/aurora

FROM alpine:3.19 AS runtime-base
RUN apk add --no-cache ca-certificates docker-cli
WORKDIR /app

# Copy configuration files
COPY configs/ /app/configs/

FROM runtime-base AS nova-runtime
COPY --from=builder /out/nova /usr/local/bin/nova
EXPOSE 9001
CMD ["nova", "daemon", "--config", "configs/nova.json", "--http", ":9001"]

FROM runtime-base AS comet-runtime
COPY --from=builder /out/comet /usr/local/bin/comet
EXPOSE 9090
CMD ["comet", "daemon", "--config", "configs/nova.json", "--grpc", ":9090"]

FROM runtime-base AS zenith-runtime
COPY --from=builder /out/zenith /usr/local/bin/zenith
EXPOSE 9000
CMD ["zenith", "serve", "--listen", ":9000", "--nova-url", "http://nova:9001", "--comet-grpc", "comet:9090"]

FROM runtime-base AS corona-runtime
COPY --from=builder /out/corona /usr/local/bin/corona
CMD ["corona", "daemon", "--config", "configs/nova.json", "--comet-grpc", "comet:9090"]

FROM runtime-base AS nebula-runtime
COPY --from=builder /out/nebula /usr/local/bin/nebula
CMD ["nebula", "daemon", "--config", "configs/nova.json", "--comet-grpc", "comet:9090"]

FROM runtime-base AS aurora-runtime
COPY --from=builder /out/aurora /usr/local/bin/aurora
EXPOSE 9002
CMD ["aurora", "daemon", "--config", "configs/nova.json", "--listen", ":9002"]

FROM nova-runtime AS default-runtime
