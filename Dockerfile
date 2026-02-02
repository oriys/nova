FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.cn,direct go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o nova ./cmd/nova

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/nova /usr/local/bin/nova
EXPOSE 9000
CMD ["nova", "daemon", "--http", ":9000"]
