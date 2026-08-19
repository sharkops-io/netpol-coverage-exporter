package coverage_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.com/sharkops/exporters/netpol-coverage-exporter/internal/coverage"
)

func TestMatches(t *testing.T) {
	pod := map[string]string{"app.kubernetes.io/name": "wiki", "tier": "web"}

	cases := []struct {
		name string
		sel  *metav1.LabelSelector
		want bool
	}{
		// An EMPTY selector selects every pod in the namespace. 10 of the 12
		// live CiliumNetworkPolicies are this shape, and so are most of the
		// NetworkPolicies sharkops-netpol renders.
		{"empty selects all", &metav1.LabelSelector{}, true},
		{"matching label", &metav1.LabelSelector{
			MatchLabels: map[string]string{"app.kubernetes.io/name": "wiki"}}, true},
		{"non-matching label", &metav1.LabelSelector{
			MatchLabels: map[string]string{"app.kubernetes.io/name": "netbox"}}, false},
		{"all matchLabels must match", &metav1.LabelSelector{
			MatchLabels: map[string]string{"app.kubernetes.io/name": "wiki", "tier": "db"}}, false},
		{"matchExpressions In", &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"web", "api"}}}}, true},
		{"matchExpressions NotIn", &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: "tier", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"web"}}}}, false},
		{"matchExpressions Exists", &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: "tier", Operator: metav1.LabelSelectorOpExists}}}, true},
		// nil is NOT the same as empty: LabelSelectorAsSelector(nil) returns
		// labels.Nothing(). Guarding this explicitly because getting it
		// backwards would silently report every pod as covered.
		{"nil selects nothing", nil, false},
		// A malformed selector -- here an invalid operator -- makes
		// LabelSelectorAsSelector return an error. That must report
		// not-covered, the same safe direction as every other malformed
		// shape in this package.
		{"invalid operator errors, reports not-covered", &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: "tier", Operator: "Bogus"}}}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := coverage.Matches(c.sel, pod); got != c.want {
				t.Errorf("Matches = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNormalizeCiliumSelector(t *testing.T) {
	t.Run("plain matchLabels", func(t *testing.T) {
		sel, ok := coverage.NormalizeCiliumSelector(map[string]any{
			"matchLabels": map[string]any{"app.kubernetes.io/name": "wiki"},
		})
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if sel.MatchLabels["app.kubernetes.io/name"] != "wiki" {
			t.Errorf("MatchLabels = %v", sel.MatchLabels)
		}
	})

	t.Run("k8s prefix is stripped", func(t *testing.T) {
		sel, ok := coverage.NormalizeCiliumSelector(map[string]any{
			"matchLabels": map[string]any{"k8s:app.kubernetes.io/name": "wiki"},
		})
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if _, present := sel.MatchLabels["app.kubernetes.io/name"]; !present {
			t.Errorf("k8s: prefix not stripped: %v", sel.MatchLabels)
		}
	})

	t.Run("empty selector is match-all, not unsupported", func(t *testing.T) {
		sel, ok := coverage.NormalizeCiliumSelector(map[string]any{})
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if !coverage.Matches(sel, map[string]string{"any": "pod"}) {
			t.Error("empty Cilium selector should match every pod")
		}
	})

	t.Run("reserved prefix is unsupported and reports not-covered", func(t *testing.T) {
		if _, ok := coverage.NormalizeCiliumSelector(map[string]any{
			"matchLabels": map[string]any{"reserved:host": ""},
		}); ok {
			t.Error("ok = true, want false for a reserved: selector")
		}
	})

	t.Run("bare k8s: prefix with no key is unsupported", func(t *testing.T) {
		if _, ok := coverage.NormalizeCiliumSelector(map[string]any{
			"matchLabels": map[string]any{"k8s:": "wiki"},
		}); ok {
			t.Error("ok = true, want false for a k8s: key with nothing after the colon")
		}
	})

	t.Run("non-string matchLabels value is unsupported, not coerced to empty", func(t *testing.T) {
		if _, ok := coverage.NormalizeCiliumSelector(map[string]any{
			"matchLabels": map[string]any{"app.kubernetes.io/name": 123},
		}); ok {
			t.Error("ok = true, want false for a non-string matchLabels value")
		}
	})
}
