package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/katamigrate"
)

func newMigrateFromKataCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "migrate-from-kata",
		Short: "copy an existing kata install into poomsae",
		Long: `Copy a kata data directory and its SQLite database into the poomsae data directory.

The copy includes the database, its -wal and -shm files, attachments, and backups.
The original kata directory is left in place. When the poomsae directory already
exists, the command leaves it unchanged. Per-project .kata.toml and
.kata.local.toml files in the workspace are copied to .poomsae.toml and
.poomsae.local.toml when those files are absent. The command is safe to run again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			warnLegacyEnv(cmd.ErrOrStderr())
			result, err := katamigrate.Migrate(cmd.Context(), katamigrate.Options{
				Workspace: workspace,
				Explicit:  true,
			})
			if err != nil {
				return err
			}
			return katamigrate.Print(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace whose .kata.toml files are copied (default: current directory)")
	return cmd
}

func warnLegacyEnv(w io.Writer) {
	if warning := config.LegacyEnvWarning(config.ApplyLegacyKataEnv()); warning != "" {
		fmt.Fprintln(w, warning)
	}
}

// maybeMigrateFromKata copies a kata install the first time another command
// runs. migrate-from-kata performs that copy itself so the summary is not
// printed twice.
func maybeMigrateFromKata(ctx context.Context, stderr io.Writer) {
	for _, arg := range os.Args[1:] {
		if arg == "migrate-from-kata" {
			return
		}
	}
	warnLegacyEnv(stderr)
	result, err := katamigrate.Migrate(ctx, katamigrate.Options{})
	if err != nil {
		fmt.Fprintf(stderr, "poomsae: kata migration: %v\n", err)
		return
	}
	if result.Changed() {
		_ = katamigrate.Print(stderr, result)
	}
}
