// Package coverage decides whether a pod is selected by a policy.
package coverage

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Matches reports whether podLabels satisfy sel.
//
// This delegates to LabelSelectorAsSelector rather than comparing maps by
// hand: hand-rolled matching silently ignores matchExpressions, which reports
// pods as UNCOVERED when they are covered, or worse the reverse.
//
// Note the asymmetry it inherits, which is deliberate and load-bearing:
// a nil selector matches NOTHING, an empty one matches EVERYTHING.
func Matches(sel *metav1.LabelSelector, podLabels map[string]string) bool {
	s, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		// A malformed selector cannot be shown to cover the pod, so report
		// not-covered: a false positive on the alert, never a false negative.
		return false
	}
	return s.Matches(labels.Set(podLabels))
}

// NormalizeCiliumSelector converts a CiliumNetworkPolicy endpointSelector into
// a standard LabelSelector.
//
// Cilium label keys may carry a SOURCE PREFIX -- "k8s:app=x", "any:app=x",
// "reserved:host". Everything sharkops-netpol renders is plain, because the
// chart passes podSelector through verbatim, but a hand-written policy need
// not be. "k8s:" is stripped; any other prefix returns ok=false and the caller
// treats the policy as not covering the pod, which is the safe direction.
func NormalizeCiliumSelector(raw map[string]any) (*metav1.LabelSelector, bool) {
	sel := &metav1.LabelSelector{MatchLabels: map[string]string{}}

	if ml, present := raw["matchLabels"].(map[string]any); present {
		for k, v := range ml {
			key, ok := stripSource(k)
			if !ok {
				return nil, false
			}
			s, _ := v.(string)
			sel.MatchLabels[key] = s
		}
	}

	if me, present := raw["matchExpressions"].([]any); present {
		for _, item := range me {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			key, ok := stripSource(asString(m["key"]))
			if !ok {
				return nil, false
			}
			req := metav1.LabelSelectorRequirement{
				Key:      key,
				Operator: metav1.LabelSelectorOperator(asString(m["operator"])),
			}
			for _, v := range asSlice(m["values"]) {
				req.Values = append(req.Values, asString(v))
			}
			sel.MatchExpressions = append(sel.MatchExpressions, req)
		}
	}

	return sel, true
}

// stripSource removes a "k8s:" source prefix. Any other prefix is unsupported.
func stripSource(key string) (string, bool) {
	i := strings.Index(key, ":")
	if i < 0 {
		return key, true
	}
	if key[:i] == "k8s" {
		return key[i+1:], true
	}
	return "", false
}

func asString(v any) string { s, _ := v.(string); return s }
func asSlice(v any) []any   { s, _ := v.([]any); return s }
