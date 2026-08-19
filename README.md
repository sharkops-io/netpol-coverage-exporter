# netpol-coverage-exporter

A Prometheus exporter reporting per-pod NetworkPolicy (and
CiliumNetworkPolicy) coverage on a Kubernetes cluster: for every pod, whether
it is selected by at least one policy, split by ingress and egress
direction. It talks to the Kubernetes API directly via `client-go` — no
Cilium Hubble, no service mesh required.

## Status

This repository is scaffolding. The configuration package and HTTP surface
exist and are tested; the coverage collector itself is not implemented yet.
`/metrics` currently serves only the standard Go runtime and process
collectors.

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
