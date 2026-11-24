package main

import (
	"encoding/json/v2"
	"fmt"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/spf13/cobra"
)

func federationEmbeddingRecipeCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "embedding-recipe",
		Short:   "export the exact local configured document recipe as JSON",
		Long:    "Export the exact document recipe from the selected Poomsae home's configuration. This does not inspect a remote daemon, open a vector index, or make an embedding request. Use the JSON with the root's existing project metadata API to select a producer.",
		Example: "  poomsae federation embedding-recipe",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.ReadSearchConfig()
			if err != nil {
				return err
			}
			effective := cfg.Embeddings
			if !effective.Enabled() {
				dsn, err := config.PoomsaeDSN(cmd.Context())
				if err != nil {
					return err
				}
				effective, err = applySharedEmbeddingChoice(cmd.Context(), effective, dsn)
				if err != nil {
					return err
				}
			}
			encoder, _, err := preflightEmbeddingStartup(effective, "")
			if err != nil {
				return err
			}
			if encoder == nil {
				return fmt.Errorf("embeddings are not configured in the selected Poomsae home")
			}
			recipe, err := encoder.ArtifactIdentity("", "", "")
			if err != nil {
				return err
			}
			// This is an interoperable recipe payload, consumed verbatim by the
			// strict project metadata API rather than a versioned CLI envelope.
			if err := json.MarshalWrite(cmd.OutOrStdout(), recipe); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout())
			return err
		},
	}
}
