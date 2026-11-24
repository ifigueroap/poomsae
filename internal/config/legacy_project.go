package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// LegacyProjectConfigFilename is the kata project file copied on first use.
	LegacyProjectConfigFilename = ".kata.toml"
	// LegacyLocalConfigFilename is the kata per-machine file copied on first use.
	LegacyLocalConfigFilename = ".kata.local.toml"
)

// AdoptLegacyProjectFiles copies kata project files in dir onto the poomsae
// names when those names are absent. Existing poomsae files and the original
// kata files are left untouched. A second call copies nothing.
func AdoptLegacyProjectFiles(dir string) ([]string, error) {
	var copied []string
	pairs := [][2]string{
		{LegacyProjectConfigFilename, ProjectConfigFilename},
		{LegacyLocalConfigFilename, LocalConfigFilename},
	}
	for _, pair := range pairs {
		src := filepath.Join(dir, pair[0])
		dst := filepath.Join(dir, pair[1])
		ok, err := copyFileIfAbsent(src, dst)
		if err != nil {
			return copied, fmt.Errorf("copy %s: %w", src, err)
		}
		if ok {
			copied = append(copied, dst)
			fmt.Fprintf(os.Stderr, "poomsae: migrated %s -> %s (original left in place)\n", src, dst)
		}
	}
	return copied, nil
}

func copyFileIfAbsent(src, dst string) (bool, error) {
	in, err := os.Open(src) //nolint:gosec // workspace path supplied by discovery
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()) //nolint:gosec // destination mode matches the source file
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return false, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return false, closeErr
	}
	return true, nil
}
