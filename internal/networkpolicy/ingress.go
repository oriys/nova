package networkpolicy

import (
	"fmt"
	"net"
	"strings"

	"github.com/oriys/nova/internal/domain"
)

// Caller describes the invocation source for ingress policy evaluation.
type Caller struct {
	SourceFunction string
	SourceIP       string
	Protocol       string
	Port           int
}

// EnforceIngress checks whether caller is allowed by the function's ingress rules.
// If no ingress rules are defined, access is allowed.
func EnforceIngress(functionName string, policy *domain.NetworkPolicy, caller Caller) error {
	if policy == nil || len(policy.IngressRules) == 0 {
		return nil
	}

	sourceFunction := strings.TrimSpace(caller.SourceFunction)
	sourceIPRaw := strings.TrimSpace(caller.SourceIP)
	sourceIP := net.ParseIP(sourceIPRaw)
	protocol := normalizeProtocol(caller.Protocol)
	if protocol == "" {
		protocol = "tcp"
	}
	port := caller.Port

	for _, rule := range policy.IngressRules {
		if !matchesSource(rule.Source, sourceFunction, sourceIPRaw, sourceIP) {
			continue
		}
		if !matchesProtocol(rule.Protocol, protocol) {
			continue
		}
		if !matchesPort(rule.Port, port) {
			continue
		}
		return nil
	}

	source := sourceFunction
	if source == "" {
		source = sourceIPRaw
	}
	if source == "" {
		source = "unknown"
	}

	return fmt.Errorf("ingress denied by network policy for %s (source=%s protocol=%s port=%d)", functionName, source, protocol, port)
}

func matchesSource(ruleSource, callerFunction, callerIPRaw string, callerIP net.IP) bool {
	source := strings.TrimSpace(ruleSource)
	if source == "" {
		return false
	}
	if source == "*" {
		return true
	}

	if callerFunction != "" && strings.EqualFold(source, callerFunction) {
		return true
	}

	if callerIPRaw == "" {
		return false
	}

	if ip := net.ParseIP(source); ip != nil && callerIP != nil {
		return ip.Equal(callerIP)
	}

	if _, cidr, err := net.ParseCIDR(source); err == nil && callerIP != nil {
		return cidr.Contains(callerIP)
	}

	return strings.EqualFold(source, callerIPRaw)
}

func matchesProtocol(ruleProtocol, callerProtocol string) bool {
	rule := normalizeProtocol(ruleProtocol)
	if rule == "" {
		rule = "tcp"
	}
	if rule == "*" {
		return true
	}
	return rule == callerProtocol
}

func normalizeProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "tcp", "http", "https", "grpc", "h2c":
		return "tcp"
	case "udp", "quic":
		return "udp"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func matchesPort(rulePort, callerPort int) bool {
	if rulePort <= 0 {
		return true
	}
	if callerPort <= 0 {
		return false
	}
	return rulePort == callerPort
}
