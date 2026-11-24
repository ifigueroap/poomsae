package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func resolveYesNo(cmd *cobra.Command, prompt string) error {
	if !isTTY(os.Stdin) {
		return &cliError{
			Message:  "no TTY: pass --yes to proceed noninteractively",
			Code:     "confirm_required",
			Kind:     kindConfirm,
			ExitCode: ExitConfirm,
		}
	}
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), prompt); err != nil {
		return err
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	//nolint:errcheck // EOF here means stdin closed; treat as a no.
	line, _ := reader.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return &cliError{
			Message:  "cancelled",
			Code:     "confirm_mismatch",
			Kind:     kindConfirm,
			ExitCode: ExitConfirm,
		}
	}
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			strings.ContainsRune("-_./:=,", r) {
			continue
		}
		safe = false
		break
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
