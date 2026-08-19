package main

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"gitlab.com/sharkops/exporters/exporter-common/collector"
	"gitlab.com/sharkops/exporters/netpol-coverage-exporter/internal/coverage"
)

// The whole path must produce a metric NAMED netpol_pod_covered and the
// self-monitoring series. A renamed metric breaks every alert silently.
func TestRegistryExposesCoverageAndSuccessSeries(t *testing.T) {
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

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector.New(src, 10*time.Second))

	got, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	var buf strings.Builder
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range got {
		if err := enc.Encode(mf); err != nil {
			t.Fatalf("Encode: %v", err)
		}
	}
	out := buf.String()

	for _, want := range []string{"netpol_pod_covered", "netpol_scrape_collector_success"} {
		if !strings.Contains(out, want) {
			t.Errorf("exposition missing %q\n%s", want, out)
		}
	}
}
