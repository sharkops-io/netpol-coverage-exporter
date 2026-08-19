package coverage_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.com/sharkops/exporters/netpol-coverage-exporter/internal/coverage"
)

func pod(name string, owner *metav1.OwnerReference) *corev1.Pod {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if owner != nil {
		p.OwnerReferences = []metav1.OwnerReference{*owner}
	}
	return p
}

func TestWorkloadName(t *testing.T) {
	ctrl := true

	// A Deployment pod is owned by a ReplicaSet whose name is the deployment
	// name plus a pod-template hash. That hash changes on every rollout, so
	// the ReplicaSet name is NOT stable and must be trimmed.
	got := coverage.WorkloadName(pod("grafana-77468df46c-6q9hp", &metav1.OwnerReference{
		Kind: "ReplicaSet", Name: "kube-prometheus-stack-grafana-77468df46c", Controller: &ctrl}))
	if got != "kube-prometheus-stack-grafana" {
		t.Errorf("WorkloadName = %q, want kube-prometheus-stack-grafana", got)
	}

	// A CronJob pod is owned by a Job named <cronjob>-<timestamp>.
	got = coverage.WorkloadName(pod("wikijs-database-wal-switch-29782140-gd7hk", &metav1.OwnerReference{
		Kind: "Job", Name: "wikijs-database-wal-switch-29782140", Controller: &ctrl}))
	if got != "wikijs-database-wal-switch" {
		t.Errorf("WorkloadName = %q, want wikijs-database-wal-switch", got)
	}

	// DaemonSet and StatefulSet owners are already stable.
	got = coverage.WorkloadName(pod("longhorn-csi-plugin-jrp8j", &metav1.OwnerReference{
		Kind: "DaemonSet", Name: "longhorn-csi-plugin", Controller: &ctrl}))
	if got != "longhorn-csi-plugin" {
		t.Errorf("WorkloadName = %q, want longhorn-csi-plugin", got)
	}

	// A bare pod owns itself.
	if got := coverage.WorkloadName(pod("standalone", nil)); got != "standalone" {
		t.Errorf("WorkloadName = %q, want standalone", got)
	}
}
