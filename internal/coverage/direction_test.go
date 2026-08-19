package coverage_test

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"

	"gitlab.com/sharkops/exporters/netpol-coverage-exporter/internal/coverage"
)

func TestNetworkPolicyDirections(t *testing.T) {
	// The qbittorrent-restrict-egress case: Egress ONLY. Counting this as
	// ingress coverage is the exact bug that recorded plex as protected while
	// ten workloads sat open.
	d := coverage.NetworkPolicyDirections([]networkingv1.PolicyType{networkingv1.PolicyTypeEgress})
	if d[coverage.DirectionIngress] {
		t.Error("an Egress-only policy must not count as ingress coverage")
	}
	if !d[coverage.DirectionEgress] {
		t.Error("Egress not recorded")
	}

	both := coverage.NetworkPolicyDirections([]networkingv1.PolicyType{
		networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress})
	if !both[coverage.DirectionIngress] || !both[coverage.DirectionEgress] {
		t.Errorf("both directions not recorded: %v", both)
	}
}

func TestCiliumDirections(t *testing.T) {
	cases := []struct {
		name          string
		spec          map[string]any
		wantIn, wantE bool
	}{
		// All 12 live CNPs are this shape.
		{"ingress only", map[string]any{"ingress": []any{map[string]any{}}}, true, false},
		{"egress only", map[string]any{"egress": []any{map[string]any{}}}, false, true},
		{"ingressDeny counts as ingress", map[string]any{"ingressDeny": []any{map[string]any{}}}, true, false},
		{"egressDeny counts as egress", map[string]any{"egressDeny": []any{map[string]any{}}}, false, true},
		{"both", map[string]any{
			"ingress": []any{map[string]any{}}, "egress": []any{map[string]any{}}}, true, true},
		// A CNP with a selector but no rules enforces nothing for either
		// direction and must not be counted as coverage.
		{"selector only, no rules", map[string]any{"endpointSelector": map[string]any{}}, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := coverage.CiliumDirections(c.spec)
			if d[coverage.DirectionIngress] != c.wantIn {
				t.Errorf("ingress = %v, want %v", d[coverage.DirectionIngress], c.wantIn)
			}
			if d[coverage.DirectionEgress] != c.wantE {
				t.Errorf("egress = %v, want %v", d[coverage.DirectionEgress], c.wantE)
			}
		})
	}
}
