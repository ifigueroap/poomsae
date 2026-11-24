package main

import (
	"encoding/json/v2"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistoricalMetadataJSONReferenceRoundTrip(t *testing.T) {
	for _, key := range []string{"", "example.key"} {
		for _, qualified := range []bool{false, true} {
			t.Run(fmt.Sprintf("key=%s/qualified=%t", key, qualified), func(t *testing.T) {
				env, dir, _, moved, oldRef := historicalOutputFixture(t)
				requestedRef := oldRef
				if qualified {
					requestedRef = "source-project#" + oldRef
				}
				runCLI(t, env, dir, "meta", "set", requestedRef, "example.key", "value")
				args := []string{"meta", "get", requestedRef, "--json"}
				if key != "" {
					args = append(args, key)
				}
				var result struct {
					Ref string `json:"ref"`
				}
				require.NoError(t, json.Unmarshal([]byte(runCLI(t, env, dir, args...)), &result))
				var shown struct {
					Issue struct {
						UID string `json:"uid"`
					} `json:"issue"`
				}
				require.NoError(t, json.Unmarshal([]byte(runCLI(t, env, dir, "show", result.Ref, "--json")), &shown))
				require.Equal(t, moved.UID, shown.Issue.UID, "metadata JSON must retain a usable reference from the source workspace")
				require.Equal(t, requestedRef, result.Ref)
			})
		}
	}
}
