// Package config resolves the poomsae data directory, database path, and
// per-database runtime namespace from environment variables.
package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PoomsaeHome returns the resolved data directory honoring $POOMSAE_HOME, falling back
// to $HOME/.poomsae. The directory is not created here; callers materialize it.
func PoomsaeHome() (string, error) {
	if v := os.Getenv("POOMSAE_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".poomsae"), nil
}

// PoomsaeDB returns the effective database DSN. It now delegates to PoomsaeDSN so
// existing callers pick up the full precedence chain (POOMSAE_DSN env >
// [storage].dsn in config.toml > legacy POOMSAE_DB env > default
// <PoomsaeHome>/poomsae.db). The returned value is whatever the user supplied: a
// bare filesystem path (default and POOMSAE_DB cases) or a sqlite/postgres DSN
// (POOMSAE_DSN / [storage].dsn cases).
//
// Kept under the old name so call sites and external scripts that spell it
// the older way continue to resolve through the same path; new code should
// prefer PoomsaeDSN.
func PoomsaeDB() (string, error) { return PoomsaeDSN(context.Background()) }

// DBHash returns the first 12 lower-hex chars of sha256(absolute(dbPath)).
// Used to namespace runtime files, sockets, and hook output per database.
// A postgres:// DSN is hashed by its credential-free effective connection
// targets so runtime files never derive from a password or collide across
// host, port, database, or fallback routing differences.
//
// A sqlite:// DSN is reduced to the same filesystem path that storeopen
// opens before hashing, so /path/poomsae.db and sqlite:///path/poomsae.db share a
// runtime namespace and `poomsae daemon` running-daemon checks. Without this
// normalization the two forms produce different hashes and the second one
// could start a duplicate daemon against the same database.
func DBHash(dbPath string) string {
	if strings.HasPrefix(dbPath, "postgres://") || strings.HasPrefix(dbPath, "postgresql://") {
		identity, err := postgresTargetIdentity(dbPath)
		if err != nil {
			// Never hash a raw postgres DSN — it may carry credentials. Fall
			// back to the redacted form so a malformed DSN still produces a
			// stable, credential-free hash.
			identity = RedactDSN(dbPath)
		}
		// A postgres identity is credential-free and is not a filesystem path,
		// so it must not pass through filepath.Abs.
		sum := sha256.Sum256([]byte(identity))
		return hex.EncodeToString(sum[:])[:12]
	}
	path := strings.TrimPrefix(dbPath, "sqlite://")
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:])[:12]
}

// StorageHash identifies one logical storage target. PostgreSQL schemas are
// independent databases from Poomsae's point of view even when they share a DSN,
// so their runtime sockets and control files must not collide.
func StorageHash(dsn, postgresSchema string) string {
	hash, _ := storageHash(dsn, postgresSchema)
	return hash
}

// storageHash also reports ErrPostgresRoutingUnavailable. Like any other
// unparsable DSN, the hash then falls back to the redacted DSN, which does not
// match the daemon's runtime namespace.
func storageHash(dsn, postgresSchema string) (string, error) {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return DBHash(dsn), nil
	}
	schema := strings.TrimSpace(postgresSchema)
	if schema == "" {
		schema = "poomsae"
	}
	identity, err := postgresTargetIdentity(dsn)
	if err != nil {
		identity = RedactDSN(dsn)
	}
	sum := sha256.Sum256([]byte(identity + "\x00schema=" + schema))
	hash := hex.EncodeToString(sum[:])[:12]
	if errors.Is(err, ErrPostgresRoutingUnavailable) {
		return hash, err
	}
	return hash, nil
}

// RuntimeDir returns <PoomsaeHome>/runtime/<dbhash>. The directory is not created.
func RuntimeDir() (string, error) {
	home, err := PoomsaeHome()
	if err != nil {
		return "", err
	}
	db, err := PoomsaeDB()
	if err != nil {
		return "", err
	}
	pgConfig, err := PoomsaePostgresStorageConfig(context.Background())
	if err != nil {
		return "", err
	}
	hash, err := storageHash(db, pgConfig.Schema)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "runtime", hash), nil
}

// HookConfigPath returns <PoomsaeHome>/hooks.toml. The file is not created here.
func HookConfigPath() (string, error) {
	home, err := PoomsaeHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "hooks.toml"), nil
}

// DaemonConfigPath returns <PoomsaeHome>/config.toml — the optional
// daemon-side settings file. Today only `listen = "..."` is honored
// (used by `poomsae daemon start` when --listen is not passed). The file
// is not created here.
func DaemonConfigPath() (string, error) {
	home, err := PoomsaeHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config.toml"), nil
}

// FederationCredentialsPath returns <PoomsaeHome>/credentials.toml. It stores
// local secrets such as federation bearer tokens and must not be committed to a
// workspace.
func FederationCredentialsPath() (string, error) {
	home, err := PoomsaeHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "credentials.toml"), nil
}

// HookRootDir returns <PoomsaeHome>/hooks/<dbhash>. Per-DB so multiple poomsae
// databases on the same host don't share output streams. Rejects any
// dbhash that is not a 12-char lower-hex string so a malformed value
// can't escape the hook root via "..", separators, or padding.
func HookRootDir(dbhash string) (string, error) {
	if err := validateDBHash(dbhash); err != nil {
		return "", err
	}
	home, err := PoomsaeHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "hooks", dbhash), nil
}

// validateDBHash enforces the shape produced by DBHash: exactly 12 chars
// of lower-hex. Rejects any input outside that contract before joining
// it into a path.
func validateDBHash(dbhash string) error {
	if len(dbhash) != 12 {
		return fmt.Errorf("dbhash %q: must be 12 chars (got %d)", dbhash, len(dbhash))
	}
	for _, r := range dbhash {
		isDigit := r >= '0' && r <= '9'
		isHexLetter := r >= 'a' && r <= 'f'
		if !isDigit && !isHexLetter {
			return fmt.Errorf("dbhash %q: must be lower-hex", dbhash)
		}
	}
	return nil
}

// HookOutputDir returns <PoomsaeHome>/hooks/<dbhash>/output. Holds per-run
// .out and .err files keyed by <event_id>.<hook_index>.
func HookOutputDir(dbhash string) (string, error) {
	root, err := HookRootDir(dbhash)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "output"), nil
}

// HookRunsPath returns <PoomsaeHome>/hooks/<dbhash>/runs.jsonl, the active
// (non-rotated) JSONL log of finished runs. Rotated copies live alongside
// as runs.jsonl.1, runs.jsonl.2, ...
func HookRunsPath(dbhash string) (string, error) {
	root, err := HookRootDir(dbhash)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "runs.jsonl"), nil
}

// MCPEventsKeyPath returns <home>/runtime/<storageID>/mcp-events.key, the
// owner-only file holding the daemon's MCP Events encryption key. It sits in
// the same runtime directory as the other per-storage control files and is
// not created here.
func MCPEventsKeyPath(home, storageID string) string {
	return filepath.Join(home, "runtime", storageID, "mcp-events.key")
}
