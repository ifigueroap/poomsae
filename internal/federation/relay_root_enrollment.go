package federation

import (
	"context"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/federationcoord"
)

// PinRelayRootAuthority validates and installs the trusted enrollment pin using
// the same helper as local replica setup, without coupling the sync and daemon.
func PinRelayRootAuthority(ctx context.Context, store db.Storage, handshake api.RelayHandshake) error {
	return federationcoord.PinRelayRootAuthority(ctx, store, handshake)
}
