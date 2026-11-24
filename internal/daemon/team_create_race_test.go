package daemon_test

import (
	"context"
	"sync/atomic"

	"github.com/ifigueroap/poomsae/internal/db"
)

type teamCreateBarrierStore struct {
	db.Storage
	arrived atomic.Int32
	ready   chan struct{}
}

func (s *teamCreateBarrierStore) CreateTeam(ctx context.Context, name, actor string) (db.Team, db.Event, error) {
	if name == "concurrent-team" {
		if s.arrived.Add(1) == 2 {
			close(s.ready)
		}
		select {
		case <-s.ready:
		case <-ctx.Done():
			return db.Team{}, db.Event{}, ctx.Err()
		}
	}
	return s.Storage.CreateTeam(ctx, name, actor)
}

// Review16802: concurrent calls meet before the native create. Uniqueness
// must produce a stable conflict for the loser, never an internal error.
