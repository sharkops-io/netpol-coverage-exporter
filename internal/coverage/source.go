package coverage

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/sharkops-io/exporter-common/metric"
)

// CiliumNetworkPolicyGVR is read through the DYNAMIC client rather than
// Cilium's generated clientset deliberately: importing github.com/cilium/cilium
// to read two fields would pull the entire agent's dependency tree into an
// exporter that needs endpointSelector and the four rule keys.
//
// CiliumClusterwideNetworkPolicy is NOT read. The CRD exists with zero objects;
// CCNPs are Phase 3. The policies slice built in the policies() method is a
// LIST so adding one is a source, not a rewrite.
var CiliumNetworkPolicyGVR = schema.GroupVersionResource{
	Group: "cilium.io", Version: "v2", Resource: "ciliumnetworkpolicies",
}

// Source emits per-pod policy coverage.
type Source struct {
	Pods    kubernetes.Interface
	Dynamic dynamic.Interface
}

// Namespace is the metric prefix, making the self-monitoring series
// netpol_scrape_collector_success and netpol_scrape_collector_duration_seconds.
func (s *Source) Namespace() string { return "netpol" }

// policy is one selector plus the directions it governs, in one namespace.
type policy struct {
	namespace  string
	selector   *metav1.LabelSelector
	directions map[Direction]bool
}

// Collect lists every pod and every policy, then joins them.
func (s *Source) Collect(ctx context.Context) ([]metric.Sample, error) {
	policies, err := s.policies(ctx)
	if err != nil {
		return nil, err
	}

	pods, err := s.Pods.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	samples := make([]metric.Sample, 0, len(pods.Items)*2)
	for i := range pods.Items {
		pod := &pods.Items[i]

		// Terminated pods are SKIPPED, not emitted-and-labelled like
		// hostNetwork ones. The distinction matters and is not arbitrary: a
		// hostNetwork pod is running and reachable, and excluding it is a
		// POLICY decision that therefore belongs in the alert expression where
		// it can be audited. A Succeeded or Failed pod has no network identity
		// at all and cannot receive a connection, so counting it as an
		// uncovered exposure is simply WRONG -- that is the metric being
		// incorrect, not a judgement about scope.
		//
		// Found by using the measure: on 2026-08-19 nine of the thirty-four
		// pods it reported as uncovered were completed CronJob runs
		// (wal-switch, housekeeping). All three of wikijs' "gaps" were dead
		// pods, and a policy written to satisfy them would have selected
		// nothing that could ever receive traffic.
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		for _, dir := range []Direction{DirectionIngress, DirectionEgress} {
			covered := 0.0
			for _, p := range policies {
				if p.namespace != pod.Namespace || !p.directions[dir] {
					continue
				}
				if Matches(p.selector, pod.Labels) {
					covered = 1
					break
				}
			}
			samples = append(samples, metric.Gauge(
				"netpol_pod_covered",
				"1 if the pod is selected by at least one NetworkPolicy or CiliumNetworkPolicy governing this direction, 0 otherwise.",
				covered,
				map[string]string{
					"namespace":    pod.Namespace,
					"pod":          pod.Name,
					"workload":     WorkloadName(pod),
					"direction":    string(dir),
					"host_network": strconv.FormatBool(pod.Spec.HostNetwork),
				},
			))
		}
	}
	return samples, nil
}

// policies gathers both policy kinds. A failure in EITHER source fails the
// whole scrape rather than returning partial data: a partial list would report
// pods as uncovered and fire a false alert, and exporter-common already
// publishes netpol_scrape_collector_success=0 for a failed collection.
func (s *Source) policies(ctx context.Context) ([]policy, error) {
	var out []policy

	nps, err := s.Pods.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list networkpolicies: %w", err)
	}
	for i := range nps.Items {
		np := &nps.Items[i]
		sel := np.Spec.PodSelector
		out = append(out, policy{
			namespace:  np.Namespace,
			selector:   &sel,
			directions: NetworkPolicyDirections(np.Spec.PolicyTypes),
		})
	}

	cnps, err := s.Dynamic.Resource(CiliumNetworkPolicyGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list ciliumnetworkpolicies: %w", err)
	}
	for _, item := range cnps.Items {
		spec, _ := item.Object["spec"].(map[string]any)
		if spec == nil {
			continue
		}
		rawSel, _ := spec["endpointSelector"].(map[string]any)
		if rawSel == nil {
			rawSel = map[string]any{}
		}
		sel, ok := NormalizeCiliumSelector(rawSel)
		if !ok {
			// Unsupported label source. Skipping reports pods as uncovered,
			// which is a false positive on the alert -- the safe direction.
			continue
		}
		out = append(out, policy{
			namespace:  item.GetNamespace(),
			selector:   sel,
			directions: CiliumDirections(spec),
		})
	}

	return out, nil
}
