package networkpolicy

import (
	"fmt"
	"net"
	"strings"

	"github.com/oriys/nova/internal/domain"
)

// EgressTarget describes an outbound connection attempt from a function VM.
type EgressTarget struct {
	Host     string
	Port     int
	Protocol string
}

// EnforceEgress checks whether an outbound connection from a function is allowed.
// If the policy is nil or has no egress rules, access is allowed by default.
// When DenyExternalAccess is set, only RFC 1918 private destinations are permitted
// unless explicitly whitelisted.
func EnforceEgress(functionName string, policy *domain.NetworkPolicy, target EgressTarget) error {
	if policy == nil {
		return nil
	}

	host := strings.TrimSpace(target.Host)
	protocol := normalizeProtocol(target.Protocol)
	if protocol == "" {
		protocol = "tcp"
	}
	port := target.Port

	// Deny non-private destinations when DenyExternalAccess is enabled
	if policy.DenyExternalAccess {
		ip := net.ParseIP(host)
		if ip != nil && !isPrivateIP(ip) {
			// Check if explicitly whitelisted in egress rules
			if !matchesAnyEgressRule(policy.EgressRules, host, ip, protocol, port) {
				return fmt.Errorf("egress denied: external access blocked for %s (dest=%s)", functionName, host)
			}
			return nil
		}
	}

	// If no egress rules are defined, access is allowed
	if len(policy.EgressRules) == 0 {
		return nil
	}

	// Check against egress rules
	ip := net.ParseIP(host)
	if matchesAnyEgressRule(policy.EgressRules, host, ip, protocol, port) {
		return nil
	}

	return fmt.Errorf("egress denied by network policy for %s (dest=%s protocol=%s port=%d)", functionName, host, protocol, port)
}

func matchesAnyEgressRule(rules []domain.EgressRule, host string, ip net.IP, protocol string, port int) bool {
	for _, rule := range rules {
		if matchesEgressHost(rule.Host, host, ip) &&
			matchesProtocol(rule.Protocol, protocol) &&
			matchesPort(rule.Port, port) {
			return true
		}
	}
	return false
}

func matchesEgressHost(ruleHost, targetHost string, targetIP net.IP) bool {
	rule := strings.TrimSpace(ruleHost)
	if rule == "" {
		return false
	}
	if rule == "*" {
		return true
	}

	// Exact hostname/IP match
	if strings.EqualFold(rule, targetHost) {
		return true
	}

	// IP match
	if ruleIP := net.ParseIP(rule); ruleIP != nil && targetIP != nil {
		return ruleIP.Equal(targetIP)
	}

	// CIDR match
	if _, cidr, err := net.ParseCIDR(rule); err == nil && targetIP != nil {
		return cidr.Contains(targetIP)
	}

	// Wildcard subdomain match (e.g., "*.example.com" matches "api.example.com")
	if strings.HasPrefix(rule, "*.") {
		suffix := rule[1:] // ".example.com"
		return strings.HasSuffix(strings.ToLower(targetHost), strings.ToLower(suffix))
	}

	return false
}

// isPrivateIP checks if an IP address is in RFC 1918 private ranges.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
	}
	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
