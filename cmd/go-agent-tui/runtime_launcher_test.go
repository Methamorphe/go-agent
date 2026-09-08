package main

import (
	"reflect"
	"testing"

	"github.com/Methamorphe/go-agent/internal/config"
)

func TestRuntimeDaemonArgsPreserveResolvedRuntimeConfig(t *testing.T) {
	cfg := config.Config{
		DataDir:        "/tmp/go-agent-data",
		ControlAddress: "/tmp/go-agent-data/control.sock",
		LogLevel:       "debug",
		PprofAddress:   "127.0.0.1:6060",
	}

	got := runtimeDaemonArgs(cfg)
	want := []string{
		"--data-dir", "/tmp/go-agent-data",
		"--control-address", "/tmp/go-agent-data/control.sock",
		"--log-level", "debug",
		"--pprof-address", "127.0.0.1:6060",
		"--runtime-daemon",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime args mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestRuntimeDaemonArgsOmitEmptyPprofAddress(t *testing.T) {
	cfg := config.Config{
		DataDir:        "/tmp/go-agent-data",
		ControlAddress: "/tmp/go-agent-data/control.sock",
		LogLevel:       "info",
	}

	got := runtimeDaemonArgs(cfg)
	for _, value := range got {
		if value == "--pprof-address" {
			t.Fatal("empty pprof address must not be forwarded")
		}
	}
	if got[len(got)-1] != "--runtime-daemon" {
		t.Fatalf("last runtime arg = %q", got[len(got)-1])
	}
}
