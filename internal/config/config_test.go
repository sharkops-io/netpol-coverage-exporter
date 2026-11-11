package config_test

import (
	"testing"

	"github.com/sharkops-io/netpol-coverage-exporter/internal/config"
)

func TestDefaultListenAddress(t *testing.T) {
	cfg, err := config.Parse(nil, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.ListenAddress != ":9105" {
		t.Errorf("ListenAddress = %q, want %q", cfg.ListenAddress, ":9105")
	}
}

func TestKubeconfigFromEnvironment(t *testing.T) {
	env := func(k string) string {
		if k == "KUBECONFIG" {
			return "/tmp/kubeconfig"
		}
		return ""
	}
	cfg, err := config.Parse(nil, env)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.KubeconfigPath != "/tmp/kubeconfig" {
		t.Errorf("KubeconfigPath = %q, want /tmp/kubeconfig", cfg.KubeconfigPath)
	}
}

func TestFlagBeatsEnvironment(t *testing.T) {
	env := func(k string) string {
		if k == "KUBECONFIG" {
			return "/tmp/from-env"
		}
		return ""
	}
	cfg, err := config.Parse([]string{"--kubeconfig=/tmp/from-flag"}, env)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.KubeconfigPath != "/tmp/from-flag" {
		t.Errorf("KubeconfigPath = %q, want /tmp/from-flag", cfg.KubeconfigPath)
	}
}
