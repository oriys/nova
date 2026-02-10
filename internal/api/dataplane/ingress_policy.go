package dataplane

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/networkpolicy"
	"google.golang.org/grpc/metadata"
)

func (h *Handler) enforceIngressPolicy(ctx context.Context, r *http.Request, fn *domain.Function) error {
	caller := ingressCallerFromRequest(ctx, r)
	return networkpolicy.EnforceIngress(fn.Name, fn.NetworkPolicy, caller)
}

func ingressCallerFromRequest(ctx context.Context, r *http.Request) networkpolicy.Caller {
	caller := networkpolicy.Caller{
		SourceFunction: strings.TrimSpace(r.Header.Get("X-Nova-Source-Function")),
		SourceIP:       firstIP(strings.TrimSpace(r.Header.Get("X-Nova-Source-IP"))),
		Protocol:       strings.TrimSpace(r.Header.Get("X-Nova-Source-Protocol")),
		Port:           parsePositivePort(strings.TrimSpace(r.Header.Get("X-Nova-Source-Port"))),
	}

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if caller.SourceFunction == "" {
			caller.SourceFunction = metadataValue(md, "x-nova-source-function")
		}
		if caller.SourceIP == "" {
			caller.SourceIP = firstIP(metadataValue(md, "x-nova-source-ip"))
		}
		if caller.Protocol == "" {
			caller.Protocol = metadataValue(md, "x-nova-source-protocol")
		}
		if caller.Port <= 0 {
			caller.Port = parsePositivePort(metadataValue(md, "x-nova-source-port"))
		}
	}

	if caller.SourceIP == "" {
		caller.SourceIP = firstIP(strings.TrimSpace(r.Header.Get("X-Forwarded-For")))
	}
	if caller.SourceIP == "" {
		caller.SourceIP = remoteAddrIP(r.RemoteAddr)
	}
	if caller.Protocol == "" {
		caller.Protocol = "tcp"
	}
	if caller.Port <= 0 {
		caller.Port = requestPort(r)
	}

	return caller
}

func metadataValue(md metadata.MD, key string) string {
	values := md.Get(strings.ToLower(strings.TrimSpace(key)))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parsePositivePort(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 || n > 65535 {
		return 0
	}
	return n
}

func firstIP(raw string) string {
	for _, part := range strings.Split(raw, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		host := candidate
		if strings.Contains(candidate, ":") {
			if parsedHost, _, err := net.SplitHostPort(candidate); err == nil {
				host = parsedHost
			}
		}
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func remoteAddrIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String()
	}
	return ""
}

func requestPort(r *http.Request) int {
	if p := parsePositivePort(strings.TrimSpace(r.Header.Get("X-Forwarded-Port"))); p > 0 {
		return p
	}
	host := strings.TrimSpace(r.Host)
	if host != "" {
		if _, rawPort, err := net.SplitHostPort(host); err == nil {
			if p := parsePositivePort(rawPort); p > 0 {
				return p
			}
		}
	}
	if r.TLS != nil {
		return 443
	}
	return 80
}
