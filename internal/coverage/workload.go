package coverage

import (
	"regexp"

	corev1 "k8s.io/api/core/v1"
)

// replicaSetSuffix is the pod-template hash a Deployment appends to its
// ReplicaSet names. jobSuffix is the schedule timestamp a CronJob appends.
var (
	replicaSetSuffix = regexp.MustCompile(`-[a-z0-9]{6,10}$`)
	jobSuffix        = regexp.MustCompile(`-\d{6,}$`)
)

// WorkloadName returns a stable identifier for the thing that created the pod.
//
// Pod names churn on every rollout, and a per-pod series with a long `for:`
// is defeated by that churn -- the alert's range measures the CURRENT series'
// life, not wall-clock time, so a restart mid-failure resets it and the
// condition is never satisfied. Aggregating on a stable workload name is what
// makes the alert survive a rollout.
//
// Resolution stops at the pod's direct controller and trims the generated
// suffix, rather than following the reference up to the Deployment or CronJob:
// that would need RBAC on every controller kind, and the trimmed name is
// already the identifier a human recognises.
func WorkloadName(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		switch ref.Kind {
		case "ReplicaSet":
			return replicaSetSuffix.ReplaceAllString(ref.Name, "")
		case "Job":
			return jobSuffix.ReplaceAllString(ref.Name, "")
		default:
			// DaemonSet, StatefulSet and bare controllers are already stable.
			return ref.Name
		}
	}
	return pod.Name
}
