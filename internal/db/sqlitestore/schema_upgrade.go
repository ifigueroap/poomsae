package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ifigueroap/poomsae/internal/db"
)

// inPlaceUpgrades drop retired indexes, rename columns, clear derived tables,
// then add canonical tables and canonical indexes. A table is added only when
// it starts empty, so no existing row needs converting. SQLite rewrites
// constraints and indexes that name a renamed column. An index that a later
// step drops is no longer canonical, so an upgrade that runs both steps never
// creates it.
var inPlaceUpgrades = []struct {
	From, To      int
	Tables        []string
	Indexes       []string
	DropIndexes   []string
	RenameColumns []columnRename
	ClearTables   []string
}{
	{41, 42, nil, []string{"idx_issues_project_created", "idx_issues_created"}, nil, nil, nil},
	{42, 43, nil, []string{"idx_events_resolved_issue_uid"}, nil, nil, nil},
	{43, 44, nil, []string{"idx_events_reset_from"}, nil, nil, nil},
	{44, 45, nil, []string{"idx_issues_uid_prefix", "idx_issues_project_uid_prefix"}, nil, nil, nil},
	{45, 46, nil, []string{"idx_issues_project_ready_priority", "idx_issues_ready_priority"}, nil, nil, nil},
	{46, 47, nil, []string{"idx_issues_ready_updated"}, nil, nil, nil},
	{47, 48, nil, []string{"idx_events_moved_from_id"}, nil, nil, nil},
	{48, 49, nil, []string{"idx_events_cron_run_uid"}, nil, nil, nil},
	{49, 50, []string{"mcp_event_subscriptions"}, []string{"idx_mcp_event_subscriptions_due", "idx_mcp_event_subscriptions_principal_callback", "idx_mcp_event_subscriptions_ended"}, nil, nil, nil},
	{50, 51, nil, []string{"idx_events_issue_uid_id"}, []string{"idx_events_resolved_issue_uid"}, nil, nil},
	{51, 52, nil, nil, nil, nil, nil},
	{52, 53, nil, []string{"idx_events_project_moves"}, nil, nil, nil},
	{53, 54, nil, nil, nil, nil, nil},
	// Embedding artifacts all predate the poomsae digest and recipe names.
	{54, 55, nil, nil, nil, []columnRename{
		{"external_field_mappings", "kata_field", "poomsae_field"},
		{"external_field_states", "conflict_kata", "conflict_poomsae"},
	}, []string{"federation_embedding_artifacts"}},
}

type columnRename struct{ Table, From, To string }

// UpgradesInPlace reports whether in-place steps reach the current schema.
func UpgradesInPlace(version int) bool {
	current := db.CurrentSchemaVersion()
	if version >= current {
		return false
	}
	for _, step := range inPlaceUpgrades {
		if step.From == version {
			if step.To <= version || step.To > current {
				return false
			}
			version = step.To
		}
	}
	return version == current
}

// UpgradeInPlace adds canonical tables and indexes and stamps their version
// atomically.
// The caller must hold the source's cutover lock.
func UpgradeInPlace(ctx context.Context, path string) error {
	sdb, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=rw&_txlock=immediate&_pragma=busy_timeout(%d)", sqliteURIPath(path), sqliteBusyTimeout.Milliseconds()))
	if err != nil {
		return fmt.Errorf("open index upgrade: %w", err)
	}
	defer func() { _ = sdb.Close() }()
	s := &Store{DB: sdb}
	return s.RetryTransient(ctx, func() error {
		tx, err := sdb.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin index upgrade: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
		var version int
		if err := tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
			return fmt.Errorf("read index upgrade version: %w", err)
		}
		if version == db.CurrentSchemaVersion() {
			return nil
		}
		if !UpgradesInPlace(version) {
			return fmt.Errorf("no in-place upgrade path from schema_version %d to %d", version, db.CurrentSchemaVersion())
		}
		tables, indexes, err := canonicalSQL(ctx)
		if err != nil {
			return err
		}
		// retired holds every index any remaining step drops, regardless of
		// order. This assumes no step re-adds a name an earlier step dropped;
		// such a step would be skipped here and the index never created.
		retired := map[string]bool{}
		for _, step := range inPlaceUpgrades {
			if step.From >= version {
				for _, name := range step.DropIndexes {
					retired[name] = true
				}
			}
		}
		skipped := map[string]bool{}
		for _, step := range inPlaceUpgrades {
			if step.From < version {
				continue
			}
			for _, name := range step.DropIndexes {
				if skipped[name] {
					continue
				}
				if _, err := tx.ExecContext(ctx, `DROP INDEX `+name); err != nil {
					return fmt.Errorf("drop index %s: %w", name, err)
				}
			}
			for _, rename := range step.RenameColumns {
				if _, err := tx.ExecContext(ctx, `ALTER TABLE `+rename.Table+` RENAME COLUMN `+rename.From+` TO `+rename.To); err != nil {
					return fmt.Errorf("rename column %s.%s: %w", rename.Table, rename.From, err)
				}
			}
			for _, name := range step.ClearTables {
				if _, err := tx.ExecContext(ctx, `DELETE FROM `+name); err != nil { // #nosec G202 -- name comes from the fixed inPlaceUpgrades table.
					return fmt.Errorf("clear table %s: %w", name, err)
				}
			}
			for _, name := range step.Tables {
				ddl, ok := tables[name]
				if !ok {
					return fmt.Errorf("canonical table %s is missing", name)
				}
				if _, err := tx.ExecContext(ctx, ddl); err != nil {
					return fmt.Errorf("add table %s: %w", name, err)
				}
			}
			for _, name := range step.Indexes {
				if retired[name] {
					skipped[name] = true
					continue
				}
				ddl, ok := indexes[name]
				if !ok {
					return fmt.Errorf("canonical index %s is missing", name)
				}
				if _, err := tx.ExecContext(ctx, ddl); err != nil {
					return fmt.Errorf("add index %s: %w", name, err)
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE meta SET value=? WHERE key='schema_version'`, db.CurrentSchemaVersion()); err != nil {
			return fmt.Errorf("stamp index upgrade: %w", err)
		}
		return tx.Commit()
	})
}

// canonicalSQL reads the canonical schema's table and index DDL by name.
func canonicalSQL(ctx context.Context) (tables, indexes map[string]string, err error) {
	canonical, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, nil, fmt.Errorf("open canonical schema: %w", err)
	}
	defer func() { _ = canonical.Close() }()
	canonical.SetMaxOpenConns(1)
	if _, err := canonical.ExecContext(ctx, schemaSQL); err != nil {
		return nil, nil, fmt.Errorf("create canonical schema: %w", err)
	}
	rows, err := canonical.QueryContext(ctx, `SELECT type,name,sql FROM sqlite_master WHERE type IN ('table','index') AND sql IS NOT NULL`)
	if err != nil {
		return nil, nil, fmt.Errorf("read canonical schema: %w", err)
	}
	defer func() { _ = rows.Close() }()
	tables, indexes = map[string]string{}, map[string]string{}
	for rows.Next() {
		var kind, name, statement string
		if err := rows.Scan(&kind, &name, &statement); err != nil {
			return nil, nil, fmt.Errorf("read canonical schema entry: %w", err)
		}
		if kind == "table" {
			tables[name] = statement
		} else {
			indexes[name] = statement
		}
	}
	return tables, indexes, rows.Err()
}
