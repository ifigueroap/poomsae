package linkpath_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae/internal/linkpath"
	"github.com/stretchr/testify/require"
)

// A file directly under a filesystem root keeps its absolute path; an empty
// parent must mean the root, not the working directory.
func TestChainKeepsFileUnderFilesystemRoot(t *testing.T) {
	root := filepath.VolumeName(os.TempDir()) + string(filepath.Separator)
	path := root + "poomsae-linkpath-missing.db"

	require.Equal(t, []string{path}, linkpath.Chain(path))
}
