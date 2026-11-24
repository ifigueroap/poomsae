package main

import (
	"strings"
	"testing"

	"github.com/ifigueroap/poomsae/internal/agentguide"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentWorkflowHelpFollowsContract(t *testing.T) {
	help := string(executeRoot(t, newRootCmd(), "--help"))
	_, workflow, found := strings.Cut(help, "Agent workflow (add --agent to each command):\n")
	require.True(t, found, "root help must include the agent workflow")
	require.True(t, strings.HasPrefix(workflow, "  poomsae agent setup "))
	workflow, _, found = strings.Cut(workflow, "Full agent contract:")
	require.True(t, found, "the workflow must link to the full contract")

	for _, surface := range []string{workflow, agentguide.Contract} {
		assert.Contains(t, surface, `poomsae search "<one or two distinctive words>" --agent`)
		assert.Contains(t, surface, `--test "<command>" --agent`)
	}

	// Valid commands can still fall outside the short workflow. Check every
	// advertised command against the contract, including nested command paths.
	commands, err := helpCommandLines(workflow, true)
	require.NoError(t, err)
	require.NotEmpty(t, commands, "workflow must advertise commands")
	for _, command := range commands {
		argv, err := splitHelpWords(command)
		require.NoError(t, err)
		cmd, _, err := newRootCmd().Find(argv[1:])
		require.NoError(t, err)
		if cmd.CommandPath() != "poomsae agent setup" {
			assert.Contains(t, agentguide.Contract, cmd.CommandPath(), "workflow command must appear in the short contract")
		}
	}

	// Prose can mention metadata without adding a command.
	assert.NotContains(t, workflow, "work.attention")
}

// The team and project-access commands belong to the administration group,
// which the root help renders for agents that rarely need it. Their help text
// and examples are validated with every other command by TestHelpExamplesParse.
