package daemon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeUILaunchURLRejectsUnsafeOrigins(t *testing.T) {
	for _, origin := range []string{
		"ftp://poomsae.example",
		"https://user@poomsae.example",
		"https://poomsae.example/prefix",
		"https://poomsae.example?tenant=alpha",
		"https://poomsae.example#fragment",
	} {
		t.Run(origin, func(t *testing.T) {
			target, ok := safeUILaunchURL(&WebSessionManager{origin: origin}, "01J00000000000000000000001")
			require.False(t, ok)
			require.Empty(t, target)
		})
	}
}
