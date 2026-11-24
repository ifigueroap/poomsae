package tui

import (
	"fmt"
	"os"
	"testing"

	"github.com/ifigueroap/poomsae/internal/client"
	"github.com/ifigueroap/poomsae/internal/storageinspect"
	"github.com/ifigueroap/poomsae/internal/testfix"
	"github.com/ifigueroap/poomsae/internal/version"
)

func TestMain(m *testing.M) {
	version.Version = "v1.2.3"
	version.Commit = "1234567"
	for _, key := range []string{"CI", "DO_NOT_TRACK", "TELEMETRY_ENABLED", "POOMSAE_TELEMETRY_ENABLED"} {
		if err := os.Unsetenv(key); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := os.Setenv("TELEMETRY_ENABLED", "1"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The poomsae binary installs its storage inspector at startup; reconnect
	// tests that restart a local profile need the same configuration.
	client.UseStorageInspector(storageinspect.Inspector{})
	os.Exit(testfix.RunIsolated(m))
}
