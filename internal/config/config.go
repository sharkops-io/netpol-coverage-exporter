// Package config parses this exporter's configuration: the common exporter
// flags plus a kubeconfig path.
package config

import (
	"io"
	"log/slog"

	"github.com/alecthomas/kingpin/v2"
	common "gitlab.com/sharkops/exporters/exporter-common/config"
)

// ErrHelpRequested is re-exported so main can treat --help as exit 0.
var ErrHelpRequested = common.ErrHelpRequested

// Config is the common base plus this exporter's own settings.
type Config struct {
	*common.Base

	// KubeconfigPath is empty in-cluster, where rest.InClusterConfig applies.
	// It exists so the exporter can be run against a cluster from a laptop,
	// which is how the known-bad-state check in the plan is performed.
	KubeconfigPath string
}

// Parse builds the configuration from args and the environment. getenv is a
// parameter rather than a call to os.Getenv so tests need no global state.
func Parse(args []string, getenv func(string) string) (*Config, error) {
	app := kingpin.New("netpol-coverage-exporter",
		"Exports per-pod NetworkPolicy and CiliumNetworkPolicy coverage.")
	app.Terminate(nil)
	app.Writer(io.Discard)

	base := common.Register(app, common.Defaults{ListenAddress: ":9105"})
	cfg := &Config{Base: base}
	app.Flag("kubeconfig", "Path to a kubeconfig. Empty means in-cluster.").
		StringVar(&cfg.KubeconfigPath)

	if err := common.Parse(app, base, args); err != nil {
		return nil, err
	}
	// Flag beats environment: Fallback only fills an EMPTY destination.
	common.Fallback(&cfg.KubeconfigPath, getenv("KUBECONFIG"))
	return cfg, nil
}

// LogHandler is promoted from Base; declared here so main imports one package.
func (c *Config) LogHandler(w io.Writer) slog.Handler { return c.Base.LogHandler(w) }
