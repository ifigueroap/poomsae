//go:build !unix

package config

import (
	"os"

	"github.com/ifigueroap/poomsae/internal/credentialfile"
)

func openEmbeddingCredentialFile(path string) (*os.File, error) {
	return credentialfile.Open(path)
}
