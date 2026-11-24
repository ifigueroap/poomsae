// Package katamigrate copies an existing kata install into the poomsae data
// directory without modifying the original.
package katamigrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	kitdaemon "go.kenn.io/kit/daemon"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/daemonruntime"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
)

const legacyDBName = "kata.db"

// Options selects the directories a migration reads and writes.
// Empty Destination and Sources use the process environment.
type Options struct {
	Destination string
	Sources     []string
	Workspace   string
	// Explicit is set for `poomsae migrate-from-kata`. A no-op then still
	// prints a summary. The first-run path stays quiet when there is nothing
	// to copy.
	Explicit bool
}

// Result describes one migration attempt.
type Result struct {
	Source      string
	Destination string
	Copied      bool
	Already     bool
	Schema      int
	Database    string
	Stopped     []int
	Projects    []string
	Warnings    []string
	Notes       []string
}

// Changed reports whether this run copied data, converted a project file, or
// stopped a daemon.
func (r Result) Changed() bool {
	return r.Copied || len(r.Projects) > 0 || len(r.Stopped) > 0 || r.Database != ""
}

// Migrate copies the first existing kata data directory onto the poomsae
// directory when the destination is absent, opens the SQLite database so
// schema migrations run, and converts project files under Workspace.
func Migrate(ctx context.Context, opts Options) (Result, error) {
	var result Result
	dest, err := destination(opts.Destination)
	if err != nil {
		return result, err
	}
	result.Destination = dest
	sources := opts.Sources
	if len(sources) == 0 {
		sources = SourceDirs()
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	source := firstExistingDir(sources)
	switch {
	case dirExists(dest):
		result.Already = true
		result.Notes = append(result.Notes, "poomsae data directory already exists; left it unchanged")
	case source == "":
		result.Notes = append(result.Notes, "no kata data directory found")
	default:
		result.Source = source
		stopped, warnings := stopDaemons(ctx, source)
		result.Stopped = stopped
		result.Warnings = append(result.Warnings, warnings...)
		if err := copyTree(ctx, source, dest); err != nil {
			return result, err
		}
		result.Copied = true
		if err := promoteDatabase(dest); err != nil {
			return result, err
		}
		if err := copyExternalDatabase(source, dest); err != nil {
			return result, err
		}
		if err := rewriteCopiedConfig(source, dest); err != nil {
			return result, err
		}
		dbPath := filepath.Join(dest, "poomsae.db")
		if info, statErr := os.Stat(dbPath); statErr == nil && info.Mode().IsRegular() {
			version, openErr := migrateSchema(ctx, dbPath)
			if openErr != nil {
				return result, openErr
			}
			result.Database = dbPath
			result.Schema = version
		}
		result.Notes = append(result.Notes, fmt.Sprintf("copied %s -> %s", source, dest))
	}
	if warn := ignoredLegacyPathEnv(); warn != "" {
		result.Warnings = append(result.Warnings, warn)
	}
	workspace := opts.Workspace
	if workspace == "" && opts.Explicit {
		workspace, err = os.Getwd()
		if err != nil {
			return result, err
		}
	}
	if workspace != "" {
		copied, adoptErr := adoptWorkspace(workspace)
		if adoptErr != nil {
			return result, adoptErr
		}
		result.Projects = append(result.Projects, copied...)
	}
	return result, nil
}

// SourceDirs lists kata data directories, preferred first. Missing paths are
// omitted. KATA_HOME wins, then the home, XDG, and Windows locations.
func SourceDirs() []string {
	var candidates []string
	if v := strings.TrimSpace(os.Getenv("KATA_HOME")); v != "" {
		candidates = append(candidates, v)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".kata"))
		if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
			candidates = append(candidates, filepath.Join(xdg, "kata"))
		} else {
			candidates = append(candidates, filepath.Join(home, ".local", "share", "kata"))
		}
		if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
			candidates = append(candidates, filepath.Join(xdg, "kata"))
		} else {
			candidates = append(candidates, filepath.Join(home, ".config", "kata"))
		}
	}
	if v := strings.TrimSpace(os.Getenv("APPDATA")); v != "" {
		candidates = append(candidates, filepath.Join(v, "kata"))
	}
	if v := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); v != "" {
		candidates = append(candidates, filepath.Join(v, "kata"))
	}
	seen := map[string]struct{}{}
	var out []string
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if dirExists(candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

func destination(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	return config.PoomsaeHome()
}

func firstExistingDir(paths []string) string {
	for _, path := range paths {
		if dirExists(path) {
			return path
		}
	}
	return ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func stopDaemons(ctx context.Context, source string) ([]int, []string) {
	runtimeRoot := filepath.Join(source, "runtime")
	entries, err := os.ReadDir(runtimeRoot)
	if err != nil {
		return nil, nil
	}
	var stopped []int
	var warnings []string
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			warnings = append(warnings, err.Error())
			return stopped, warnings
		}
		if !entry.IsDir() {
			continue
		}
		store := kitdaemon.RuntimeStore{Dir: filepath.Join(runtimeRoot, entry.Name())}
		records, err := store.List()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("read kata daemon records in %s: %v", store.Dir, err))
			continue
		}
		for _, record := range records {
			if record.PID <= 0 || !kitdaemon.ProcessAlive(record.PID) {
				continue
			}
			if err := daemonruntime.SignalDaemonStop(record, ""); err != nil {
				warnings = append(warnings, fmt.Sprintf("could not stop kata daemon pid %d: %v", record.PID, err))
				continue
			}
			stopped = append(stopped, record.PID)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for _, pid := range stopped {
		for kitdaemon.ProcessAlive(pid) && time.Now().Before(deadline) {
			if err := ctx.Err(); err != nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return stopped, warnings
}

func copyTree(ctx context.Context, src, dst string) error {
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", parent, err)
	}
	tmp, err := os.MkdirTemp(parent, ".poomsae-migrate-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(tmp, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode().IsRegular():
			return copyRegular(path, target, info.Mode().Perm())
		default:
			return nil
		}
	}); err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("place %s: %w", dst, err)
	}
	return nil
}

func copyRegular(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src) //nolint:gosec // path is inside the kata data directory being copied
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode) //nolint:gosec // mode matches the source file
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func promoteDatabase(dir string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := filepath.Join(dir, legacyDBName+suffix)
		dst := filepath.Join(dir, "poomsae.db"+suffix)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := copyRegularMode(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyRegularMode(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return copyRegular(src, dst, info.Mode().Perm())
}

func copyExternalDatabase(source, dest string) error {
	path, ok := legacyDatabaseOutside(source)
	if !ok {
		return nil
	}
	dst := filepath.Join(dest, "poomsae.db")
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := path + suffix
		if _, err := os.Stat(src); err != nil {
			continue
		}
		target := filepath.Join(dest, "poomsae.db"+suffix)
		if _, err := os.Stat(target); err == nil {
			continue
		}
		if err := copyRegularMode(src, target); err != nil {
			return err
		}
	}
	return nil
}

func legacyDatabaseOutside(source string) (string, bool) {
	raw := strings.TrimSpace(os.Getenv("KATA_DSN"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("KATA_DB"))
	}
	if raw == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "postgres://") || strings.HasPrefix(raw, "postgresql://") {
		return "", false
	}
	path := strings.TrimPrefix(raw, "sqlite://")
	if path == "" || strings.Contains(path, "://") {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sourceAbs, err := filepath.Abs(source)
	if err == nil && (abs == sourceAbs || strings.HasPrefix(abs, sourceAbs+string(os.PathSeparator))) {
		return "", false
	}
	if info, err := os.Stat(abs); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return abs, true
}

func rewriteCopiedConfig(source, dest string) error {
	path := filepath.Join(dest, "config.toml")
	data, err := os.ReadFile(path) //nolint:gosec // file inside the copy just written
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	text := string(data)
	text = strings.ReplaceAll(text, source, dest)
	text = strings.ReplaceAll(text, legacyDBName, "poomsae.db")
	if text == string(data) {
		return nil
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

func migrateSchema(ctx context.Context, path string) (int, error) {
	store, err := sqlitestore.Open(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("migrate %s: %w", path, err)
	}
	version, err := store.SchemaVersion(ctx)
	closeErr := store.Close()
	if err != nil {
		return 0, fmt.Errorf("read schema %s: %w", path, err)
	}
	if closeErr != nil {
		return 0, closeErr
	}
	return version, nil
}

func ignoredLegacyPathEnv() string {
	var names []string
	for _, key := range []string{"KATA_HOME", "KATA_DB", "KATA_DSN"} {
		if strings.TrimSpace(os.Getenv(key)) != "" && strings.TrimSpace(os.Getenv("POOMSAE_"+strings.TrimPrefix(key, "KATA_"))) == "" {
			names = append(names, key)
		}
	}
	dsn := strings.TrimSpace(os.Getenv("KATA_DSN"))
	if dsn != "" && (strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")) && strings.TrimSpace(os.Getenv("POOMSAE_DSN")) == "" {
		_ = os.Setenv("POOMSAE_DSN", dsn)
		return "poomsae: warning: KATA_DSN is deprecated; mapped it to POOMSAE_DSN"
	}
	if len(names) == 0 {
		return ""
	}
	return "poomsae: warning: " + strings.Join(names, ", ") + " is deprecated and was not pointed at the original kata files; poomsae uses its own data directory"
}

func adoptWorkspace(start string) ([]string, error) {
	dir := start
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	var copied []string
	for {
		names, err := config.AdoptLegacyProjectFiles(dir)
		if err != nil {
			return copied, err
		}
		copied = append(copied, names...)
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return copied, nil
}

// Print writes the human summary of a migration.
func Print(out io.Writer, result Result) error {
	if _, err := fmt.Fprintf(out, "poomsae data directory: %s\n", result.Destination); err != nil {
		return err
	}
	for _, note := range result.Notes {
		if _, err := fmt.Fprintln(out, note); err != nil {
			return err
		}
	}
	if result.Database != "" {
		if _, err := fmt.Fprintf(out, "sqlite database: %s (schema %d)\n", result.Database, result.Schema); err != nil {
			return err
		}
	}
	for _, pid := range result.Stopped {
		if _, err := fmt.Fprintf(out, "stopped kata daemon pid %d\n", pid); err != nil {
			return err
		}
	}
	for _, path := range result.Projects {
		if _, err := fmt.Fprintf(out, "project file: %s\n", path); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintln(out, warning); err != nil {
			return err
		}
	}
	if result.Copied || len(result.Projects) > 0 {
		if _, err := fmt.Fprintln(out, "original kata files were left in place"); err != nil {
			return err
		}
	}
	return nil
}
