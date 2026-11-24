package main

import (
	"encoding/json/v2"
	"fmt"
	"path/filepath"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/mcpdiscovery"
	"github.com/spf13/cobra"
)

func newMCPStatusCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "status",
		Short: "List running HTTP MCP listeners",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			home, err := config.PoomsaeHome()
			if err != nil {
				return err
			}
			directory := filepath.Join(home, "mcp")
			endpoints, err := mcpdiscovery.List(directory)
			if err != nil {
				return err
			}
			if currentOutputMode() == outputJSON {
				return json.MarshalWrite(command.OutOrStdout(), endpoints)
			}
			if len(endpoints) == 0 {
				_, err := fmt.Fprintln(command.OutOrStdout(), "No HTTP MCP listeners are running.")
				return err
			}
			for _, endpoint := range endpoints {
				if _, err := fmt.Fprintf(command.OutOrStdout(), "MCP %s (pid %d)\n", endpoint.URL, endpoint.PID); err != nil {
					return err
				}
			}
			return nil
		},
	}
	return command
}
