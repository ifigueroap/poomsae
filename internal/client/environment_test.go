package client

import (
	"os"
	"testing"

	"github.com/ifigueroap/poomsae/internal/storageinspect"
	"github.com/ifigueroap/poomsae/internal/testfix"
)

// The poomsae binary installs its storage inspector at startup; these tests
// exercise the same configuration.
func TestMain(m *testing.M) {
	UseStorageInspector(storageinspect.Inspector{})
	os.Exit(testfix.RunIsolated(m))
}
