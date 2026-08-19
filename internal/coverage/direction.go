package coverage

import networkingv1 "k8s.io/api/networking/v1"

// Direction is the traffic direction a policy governs.
type Direction string

const (
	DirectionIngress Direction = "ingress"
	DirectionEgress  Direction = "egress"
)

// NetworkPolicyDirections reads the explicit policyTypes field.
func NetworkPolicyDirections(pt []networkingv1.PolicyType) map[Direction]bool {
	d := map[Direction]bool{}
	for _, t := range pt {
		switch t {
		case networkingv1.PolicyTypeIngress:
			d[DirectionIngress] = true
		case networkingv1.PolicyTypeEgress:
			d[DirectionEgress] = true
		}
	}
	return d
}

// CiliumDirections infers direction from which rule keys are present.
//
// A CiliumNetworkPolicy has NO policyTypes field: Cilium enforces a direction
// only if the policy carries rules for it. A rules-free CNP therefore governs
// nothing, and a policy selecting an endpoint for egress alone leaves that
// endpoint's ingress wide open.
func CiliumDirections(spec map[string]any) map[Direction]bool {
	d := map[Direction]bool{}
	for _, k := range []string{"ingress", "ingressDeny"} {
		if v, present := spec[k]; present && v != nil {
			d[DirectionIngress] = true
		}
	}
	for _, k := range []string{"egress", "egressDeny"} {
		if v, present := spec[k]; present && v != nil {
			d[DirectionEgress] = true
		}
	}
	return d
}
