package coverage_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"gitlab.com/sharkops/exporters/exporter-common/metric"
	"gitlab.com/sharkops/exporters/netpol-coverage-exporter/internal/coverage"
)

func sampleFor(t *testing.T, samples []metric.Sample, ns, pod, dir string) (float64, bool) {
	t.Helper()
	for _, s := range samples {
		if s.Labels["namespace"] == ns && s.Labels["pod"] == pod && s.Labels["direction"] == dir {
			return s.Value, true
		}
	}
	return 0, false
}

// newEmptyDynamicClient builds a fake dynamic client with no
// CiliumNetworkPolicy objects. It still must register the GVR's list kind:
// the fake client panics on List for any GVR it cannot map to a list kind,
// even when zero objects of that kind exist.
func newEmptyDynamicClient() dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			coverage.CiliumNetworkPolicyGVR: "CiliumNetworkPolicyList",
		})
}

func newPod(ns, name string, labels map[string]string, hostNet bool) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Labels: labels},
		Spec:       corev1.PodSpec{HostNetwork: hostNet},
	}
}

// The plex regression, encoded as a test: an Egress-only NetworkPolicy must
// leave the pod reported UNCOVERED for ingress.
func TestEgressOnlyPolicyDoesNotCoverIngress(t *testing.T) {
	p := newPod("plex", "qbittorrent-abc", map[string]string{"app.kubernetes.io/name": "qbittorrent"}, false)
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "plex", Name: "qbittorrent-restrict-egress"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "qbittorrent"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		},
	}

	src := &coverage.Source{
		Pods:    k8sfake.NewSimpleClientset(p, np),
		Dynamic: newEmptyDynamicClient(),
	}
	samples, err := src.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if v, ok := sampleFor(t, samples, "plex", "qbittorrent-abc", "ingress"); !ok || v != 0 {
		t.Errorf("ingress covered = %v (found=%v), want 0", v, ok)
	}
	if v, ok := sampleFor(t, samples, "plex", "qbittorrent-abc", "egress"); !ok || v != 1 {
		t.Errorf("egress covered = %v (found=%v), want 1", v, ok)
	}
}

// A CiliumNetworkPolicy with an EMPTY endpointSelector covers every pod in its
// namespace. 10 of the 12 live CNPs are this shape.
func TestCiliumPolicyCoversIngress(t *testing.T) {
	p := newPod("openbao", "openbao-0", map[string]string{"app.kubernetes.io/name": "openbao"}, false)

	cnp := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cilium.io/v2",
		"kind":       "CiliumNetworkPolicy",
		"metadata":   map[string]any{"namespace": "openbao", "name": "openbao-restrict-cilium-ingress"},
		"spec": map[string]any{
			"endpointSelector": map[string]any{},
			"ingress":          []any{map[string]any{"fromEntities": []any{"ingress"}}},
		},
	}}

	scheme := runtime.NewScheme()
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			coverage.CiliumNetworkPolicyGVR: "CiliumNetworkPolicyList",
		}, cnp)

	src := &coverage.Source{Pods: k8sfake.NewSimpleClientset(p), Dynamic: dyn}
	samples, err := src.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if v, ok := sampleFor(t, samples, "openbao", "openbao-0", "ingress"); !ok || v != 1 {
		t.Errorf("ingress covered = %v (found=%v), want 1", v, ok)
	}
	if v, ok := sampleFor(t, samples, "openbao", "openbao-0", "egress"); !ok || v != 0 {
		t.Errorf("egress covered = %v (found=%v), want 0", v, ok)
	}
}

// hostNetwork pods are EMITTED and labelled, never dropped. Silently omitting
// them is the failure this whole exporter exists to remove; the exclusion
// belongs in the alert expression where it is auditable.
func TestHostNetworkPodsAreEmittedAndLabelled(t *testing.T) {
	p := newPod("monitoring", "node-exporter-xyz", map[string]string{"app": "ne"}, true)
	src := &coverage.Source{
		Pods:    k8sfake.NewSimpleClientset(p),
		Dynamic: newEmptyDynamicClient(),
	}
	samples, err := src.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var found bool
	for _, s := range samples {
		if s.Labels["pod"] == "node-exporter-xyz" {
			found = true
			if s.Labels["host_network"] != "true" {
				t.Errorf("host_network = %q, want true", s.Labels["host_network"])
			}
		}
	}
	if !found {
		t.Fatal("hostNetwork pod was dropped; it must be emitted and labelled")
	}
}

// A policy in ANOTHER namespace must never count as coverage.
func TestPolicyDoesNotLeakAcrossNamespaces(t *testing.T) {
	p := newPod("plex", "sonarr-abc", map[string]string{"app.kubernetes.io/name": "sonarr"}, false)
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "wikijs", Name: "wikijs"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		},
	}
	src := &coverage.Source{
		Pods:    k8sfake.NewSimpleClientset(p, np),
		Dynamic: newEmptyDynamicClient(),
	}
	samples, err := src.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if v, _ := sampleFor(t, samples, "plex", "sonarr-abc", "ingress"); v != 0 {
		t.Error("a policy in another namespace counted as coverage")
	}
}
