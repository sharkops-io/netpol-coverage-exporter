// Command netpol-coverage-exporter serves Prometheus metrics reporting
// per-pod NetworkPolicy (and CiliumNetworkPolicy) coverage on a Kubernetes
// cluster.
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"gitlab.com/sharkops/exporters/netpol-coverage-exporter/internal/config"
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

	reg := buildRegistry(cfg)

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

// buildRegistry wires the configured collectors onto a fresh registry. It is
// split out of main, rather than inlined there, so the whole config →
// registry → /metrics path is reachable from a test.
//
// This is scaffolding: only the Go/process runtime collectors are registered
// here. The coverage collector itself lands in a later task, which rewrites
// this function to wire it in.
func buildRegistry(_ *config.Config) *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return reg
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
