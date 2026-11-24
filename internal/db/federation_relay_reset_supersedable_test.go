package db_test

import (
	"testing"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/stretchr/testify/require"
)

// TestRelayDownstreamOwesRootWork covers the supersedable cases through its
// caller. This case adds what that test lacks: an event the root never
// accepted still blocks when the checkpoint attests other events.
func TestRelayDeliverySupersedableNeedsTheEventsOwnAttestation(t *testing.T) {
	rootAccepted := map[string]string{"event-a": "hash-a"}
	require.False(t, db.RelayDeliverySupersedable(db.RelayStreamEvent, "event-c", "hash-c", false, rootAccepted))
}
