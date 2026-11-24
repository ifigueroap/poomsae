package daemon_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/ifigueroap/poomsae/internal/db"
)

type failingDocbankBindingStore struct {
	db.Storage
	injected atomic.Bool
}

func (s *failingDocbankBindingStore) FederationBindingByProject(ctx context.Context, projectID int64) (db.FederationBinding, error) {
	pcs := make([]uintptr, 16)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".docbankUpstream") {
			s.injected.Store(true)
			return db.FederationBinding{}, errors.New("synthetic binding read failure")
		}
		if !more {
			break
		}
	}
	return s.Storage.FederationBindingByProject(ctx, projectID)
}

// A transient binding failure must never turn a federated root write into an
// unmarked legacy attachment or bypass its configured exposure subtree.
