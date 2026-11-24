package main

import (
	"fmt"
	"os"

	"github.com/ifigueroap/poomsae/internal/connector/forgejo"
	"github.com/ifigueroap/poomsae/pkg/connector"
	"github.com/spf13/cobra"
)

func newConnectorServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "serve a built-in connector RPC on standard input and output",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "forgejo",
		Short: "serve one Forgejo connector RPC",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := connector.ServeOne(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), forgejo.New(os.Getenv("FORGEJO_TOKEN"))); err != nil {
				return fmt.Errorf("invalid Forgejo connector request: %w", err)
			}
			return nil
		},
		// A connector invocation is an internal RPC, so it must not report CLI use.
		PersistentPostRunE: func(*cobra.Command, []string) error { return nil },
	})
	return cmd
}
