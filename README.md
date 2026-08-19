# netpol-coverage-exporter

A Prometheus exporter reporting per-pod NetworkPolicy (and
CiliumNetworkPolicy) coverage on a Kubernetes cluster: for every pod, whether
it is selected by at least one policy, split by ingress and egress
direction. It talks to the Kubernetes API directly via `client-go` — no
Cilium Hubble, no service mesh required.

## Why this exists

Nothing else exposes this. `kube-state-metrics` ships no NetworkPolicy
selector metric at all. Cilium's own
`cilium_policy_endpoint_enforcement_status` is a per-agent aggregate with no
namespace label, so it can say "enforcement is on somewhere" but never "pod X
in namespace Y is uncovered". `CiliumEndpoint.status.policy`, which used to
carry per-endpoint enforcement state, was removed from the 1.20 chart. This
exporter fills that gap by joining every pod against every NetworkPolicy and
CiliumNetworkPolicy directly, the same way `kubectl` or Cilium itself would
resolve coverage.

It exists because "coverage" was recorded complete at 21/21 namespaces while
the honest per-pod number was 56/109 — a namespace can hold a policy that
selects nothing, or an egress-only policy that leaves ingress wide open, and
neither shows up if you only count policy *objects*. This exporter counts
pods actually *selected*.

## What it does

On every scrape, `Source.Collect` lists every Pod, every `NetworkPolicy`, and
every `CiliumNetworkPolicy` (via the dynamic client — see the doc comment on
`CiliumNetworkPolicyGVR` in `internal/coverage/source.go` for why not the
generated Cilium clientset), then for each pod and each direction
(`ingress`/`egress`) checks whether any policy in the pod's namespace selects
it for that direction. `CiliumClusterwideNetworkPolicy` is deliberately not
read — the CRD exists with zero objects on this cluster and CCNPs are
Phase 3 of the default-deny rollout.

It emits one gauge per pod per direction:

```
netpol_pod_covered{namespace, pod, workload, direction, host_network} 0|1
```

| Label | Meaning |
|---|---|
| `namespace` | The pod's namespace. |
| `pod` | The pod's name. Churns on every rollout. |
| `workload` | A stable identifier derived from the pod's owning controller (`internal/coverage/WorkloadName`), with the generated ReplicaSet/Job suffix trimmed. Aggregate alerts on this, not on `pod`, so a rollout mid-failure doesn't reset the `for:` clock. This is also the label the longhorn CSI-sidecar exclusion in `netpol-coverage-rules.yaml` matches on — renaming it breaks that exclusion silently. |
| `direction` | `ingress` or `egress`. An egress-only policy leaves `direction="ingress"` at 0 for that pod — this is the specific bug ("the plex regression") this metric exists to catch. |
| `host_network` | Whether the pod uses the host's network namespace. hostNetwork pods are always emitted and labelled, never dropped — any exclusion belongs in the alert expression, where it's auditable, not silently baked into the collector. |

`1` means the pod is selected by at least one NetworkPolicy or
CiliumNetworkPolicy governing that direction; `0` means it is not. A
CiliumNetworkPolicy with an empty `endpointSelector` covers every pod in its
namespace (this is the shape of 10 of the 12 live CNPs on this cluster). A
policy in another namespace never counts, even with an identical selector.

If either policy list fails to load, the whole scrape fails rather than
returning partial data — a partial list would under-report policies and
report covered pods as uncovered, firing a false alert. The standard
`exporter-common` self-monitoring series
(`netpol_scrape_collector_success`, `netpol_scrape_collector_duration_seconds`)
report that failure.

## RBAC

Read-only, cluster-wide, no secrets:

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["cilium.io"]
    resources: ["ciliumnetworkpolicies"]
    verbs: ["get", "list", "watch"]
```

Cluster-wide because the question this exporter answers — "which pods are
uncovered" — is not answerable from inside one namespace. Omitting the
`cilium.io` rule makes every CNP-covered pod report as uncovered: a loud
false positive rather than a silent gap, but still wrong, so don't omit it.

## Quick start

```bash
go run ./cmd/netpol-coverage-exporter --kubeconfig=$HOME/.kube/config
```

In-cluster, omit `--kubeconfig` (or `KUBECONFIG`) entirely and the exporter
falls back to `rest.InClusterConfig`.

Then scrape `http://localhost:9105/metrics`.

## Flag reference

| Flag | Environment | Default | Description |
|---|---|---|---|
| `--web.listen-address` | — | `:9105` | Address to listen on. |
| `--web.telemetry-path` | — | `/metrics` | Path under which to expose metrics. |
| `--collector.timeout` | — | `25s` | Per-collector timeout for a whole scrape. Keep below Prometheus' `scrape_timeout`. |
| `--http.timeout` | — | `10s` | Per-HTTP-request timeout. |
| `--log.level` | — | `info` | `debug`, `info`, `warn` or `error`. |
| `--log.format` | — | `logfmt` | `logfmt` or `json`. |
| `--kubeconfig` | `KUBECONFIG` | — | Path to a kubeconfig. Empty means in-cluster. |

Flags always take precedence over the environment variable above, which
exists so a Helm values file can stay fully declarative.

Run `netpol-coverage-exporter --help` for the same list generated straight
from the binary.

## License

MIT — see [LICENSE](LICENSE).
