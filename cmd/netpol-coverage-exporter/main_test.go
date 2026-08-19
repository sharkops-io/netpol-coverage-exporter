package main

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"gitlab.com/sharkops/exporters/netpol-coverage-exporter/internal/coverage"
)

// The whole config -> registry -> /metrics path must produce metrics NAMED
// exactly netpol_pod_covered, netpol_scrape_collector_success, and the
// Go/process runtime series. This calls the REAL buildRegistry — not a
// hand-assembled registry — so a wrong timeout, a swapped clientset/dynamic
// client, a dropped collector.New call, or a dropped runtime-collector
// registration inside buildRegistry all fail this test. A renamed metric
// breaks the alert already shipped in the monitoring repo, whose expr
// matches the exact name, so names are asserted exactly rather than by
// substring: a substring check would let netpol_pod_covered_v2 or
// netpol_pod_covered_RENAMED pass silently.
func TestBuildRegistryExposesCoverageAndRuntimeSeries(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "plex", Name: "sonarr-abc"}}
	// The fake dynamic client panics on List for any GVR it cannot map to a
	// list kind, even with zero objects of that kind registered — so the
	// CiliumNetworkPolicy GVR must be registered here too (see
	// internal/coverage/source_test.go's newEmptyDynamicClient).
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			coverage.CiliumNetworkPolicyGVR: "CiliumNetworkPolicyList",
		})
	src := &coverage.Source{
		Pods:    k8sfake.NewSimpleClientset(pod),
		Dynamic: dyn,
	}

	reg := buildRegistry(src, 10*time.Second)

	got, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	names := make(map[string]bool, len(got))
	for _, mf := range got {
		names[mf.GetName()] = true
	}

	// netpol_pod_covered and netpol_scrape_collector_success prove the
	// coverage collector.Source made it into the registry with the right
	// name; go_goroutines and process_start_time_seconds prove the runtime
	// collectors registered in buildRegistry are still there.
	for _, want := range []string{
		"netpol_pod_covered",
		"netpol_scrape_collector_success",
		"go_goroutines",
		"process_start_time_seconds",
	} {
		if !names[want] {
			t.Errorf("exposition missing exact metric family %q; got families: %v", want, names)
		}
	}
}
