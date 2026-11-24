package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistoricalMetadataRetainsQualifiedRequestedRef(t *testing.T) {
	env, dir, _, _, oldRef := historicalOutputFixture(t)
	runCLI(t, env, dir, "meta", "set", oldRef, "example.key", "value")
	qualifiedOldRef := "source-project#" + oldRef
	output := runCLI(t, env, dir, "--project", "target-project", "meta", "get", qualifiedOldRef, "--agent")
	require.Contains(t, output, fmt.Sprintf("OK meta get %s ", qualifiedOldRef))
	require.Contains(t, output, "key=example.key value=")
}
