package main

import "github.com/ifigueroap/poomsae/internal/agentguide"

// agentsBlockBegin and agentsBlockEnd delimit the section poomsae manages inside
// an agent guidance file. They let init refresh poomsae's guidance in place across
// re-runs without disturbing anything else a project keeps in that file.
const (
	agentsBlockBegin = agentguide.BlockBegin
	agentsBlockEnd   = agentguide.BlockEnd
)

// agentContractText is the canonical marker-free briefing shared by dynamic
// session injection and repository-managed agent guidance.
const agentContractText = agentguide.Contract

// agentsManagedBlock returns the full marker-delimited block poomsae writes.
func agentsManagedBlock() string {
	block, _ := agentguide.ManagedBlock("cli")
	return block
}
