package networkpolicy

import (
	"testing"

	"github.com/oriys/nova/internal/domain"
)

func TestEnforceIngress_NoRulesAllows(t *testing.T) {
	if err := EnforceIngress("target", nil, Caller{}); err != nil {
		t.Fatalf("expected nil error for nil policy, got %v", err)
	}
	if err := EnforceIngress("target", &domain.NetworkPolicy{}, Caller{}); err != nil {
		t.Fatalf("expected nil error for empty ingress rules, got %v", err)
	}
}

func TestEnforceIngress_SourceFunctionMatch(t *testing.T) {
	policy := &domain.NetworkPolicy{
		IngressRules: []domain.IngressRule{
			{Source: "auth-fn", Port: 443, Protocol: "tcp"},
		},
	}

	err := EnforceIngress("target", policy, Caller{
		SourceFunction: "auth-fn",
		Protocol:       "https",
		Port:           443,
	})
	if err != nil {
		t.Fatalf("expected source function to match ingress rule, got %v", err)
	}
}

func TestEnforceIngress_CIDRMatch(t *testing.T) {
	policy := &domain.NetworkPolicy{
		IngressRules: []domain.IngressRule{
			{Source: "10.0.0.0/8"},
		},
	}

	err := EnforceIngress("target", policy, Caller{
		SourceIP: "10.24.8.9",
		Protocol: "tcp",
		Port:     9000,
	})
	if err != nil {
		t.Fatalf("expected CIDR source to match ingress rule, got %v", err)
	}
}

func TestEnforceIngress_PortMismatchDenied(t *testing.T) {
	policy := &domain.NetworkPolicy{
		IngressRules: []domain.IngressRule{
			{Source: "auth-fn", Port: 443, Protocol: "tcp"},
		},
	}

	err := EnforceIngress("target", policy, Caller{
		SourceFunction: "auth-fn",
		Protocol:       "tcp",
		Port:           9000,
	})
	if err == nil {
		t.Fatalf("expected ingress denial for mismatched port")
	}
}
