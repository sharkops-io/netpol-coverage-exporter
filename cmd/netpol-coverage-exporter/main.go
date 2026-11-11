// Command netpol-coverage-exporter serves Prometheus metrics reporting
// per-pod NetworkPolicy (and CiliumNetworkPolicy) coverage on a Kubernetes
// cluster.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/sharkops-io/exporter-common/collector"
	"github.com/sharkops-io/netpol-coverage-exporter/internal/config"
	"github.com/sharkops-io/netpol-coverage-exporter/internal/coverage"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Parse(os.Args[1:], os.Getenv)
	if errors.Is(err, config.ErrHelpRequested) {
		// kingpin already wrote the usage text; --help is success, not a
		// configuration error, so this exits 0 like any conventional CLI.
		os.Exit(0)
	}
	if err != nil {
		// --log.level/--log.format live in the very config that failed to
		// parse, so this one message necessarily goes out on the default handler.
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(cfg.LogHandler(os.Stderr)))

	slog.Info("starting netpol-coverage-exporter", "version", version)

	src, err := newSource(cfg)
	if err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}
	reg := buildRegistry(src, cfg.Timeout)

	mux := http.NewServeMux()
	mux.Handle(cfg.TelemetryPath, metricsHandler(reg))
	// Deliberately NO /healthz. A probe that collects would couple this pod's
	// restarts to the Kubernetes API being reachable; a broken exporter
	// should surface as a DOWN Prometheus target instead.
	slog.Info("listening", "address", cfg.ListenAddress, "path", cfg.TelemetryPath)
	if err := http.ListenAndServe(cfg.ListenAddress, mux); err != nil {
		slog.Error("http server", "err", err)
		os.Exit(1)
	}
}

// buildRegistry wires src onto a fresh registry alongside the Go/process
// runtime collectors. It takes an already-built collector.Source rather than
// a *config.Config precisely so this seam — the whole
// collector -> registry -> /metrics path — is reachable from a test with a
// fake Source, instead of requiring a real Kubernetes API server. Building
// the real clients from cfg is newSource's job, called only from main.
func buildRegistry(src collector.Source, timeout time.Duration) *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collector.New(src, timeout))
	return reg
}

// newSource builds the coverage.Source's real Kubernetes clients from cfg.
// Kept separate from buildRegistry so client construction — which needs a
// real or reachable kubeconfig and cannot be exercised with a fake — stays
// out of the tested seam.
func newSource(cfg *config.Config) (*coverage.Source, error) {
	restCfg, err := restConfig(cfg.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("kube config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("clientset: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	return &coverage.Source{Pods: clientset, Dynamic: dyn}, nil
}

// restConfig builds a Kubernetes REST config from an explicit kubeconfig path
// when one is given (running the exporter against a cluster from a laptop),
// or from the in-cluster service account otherwise.
func restConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

// metricsHandler serves reg at the telemetry path.
//
// ErrorHandling is ContinueOnError, and that is load-bearing rather than
// stylistic. The zero value is HTTPErrorOnError, under which ANY error from a
// single Gather is answered with HTTP 500 and an empty body, throwing away
// every metric on the endpoint. ContinueOnError serves everything that
// gathered cleanly with HTTP 200 and records the failure on
// promhttp_metric_handler_errors_total{cause="gathering"} instead.
//
// InstrumentMetricHandler wraps the handler to also register
// promhttp_metric_handler_requests_total (by HTTP status code) and
// promhttp_metric_handler_requests_in_flight on reg.
func metricsHandler(reg *prometheus.Registry) http.Handler {
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		Registry:      reg,
		ErrorHandling: promhttp.ContinueOnError,
	})
	return promhttp.InstrumentMetricHandler(reg, h)
}
