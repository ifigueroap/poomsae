//go:build !windows

package e2e_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type e2eFederationStatusBody struct {
	Statuses []struct {
		ActiveQuarantineCount int64  `json:"active_quarantine_count"`
		ResetBlocker          string `json:"reset_blocker,omitempty"`
	} `json:"statuses"`
}

func waitForFederatedIssueGone(t *testing.T, store *sqlitestore.Store, issueUID string, daemonStderr *safeBuffer) {
	t.Helper()
	var lastErr error
	require.Eventually(t, func() bool {
		_, err := store.IssueByUID(t.Context(), issueUID, db.IncludeDeletedYes)
		lastErr = err
		return federatedIssueGone(err)
	}, 5*time.Second, 50*time.Millisecond,
		"federated issue %s remained after reset; last lookup error: %v; daemon stderr: %s",
		issueUID, lastErr, daemonStderr.String())
}

func federatedIssueGone(err error) bool {
	return errors.Is(err, db.ErrNotFound)
}

func TestFederatedIssueGoneOnlyAcceptsNotFound(t *testing.T) {
	assert.True(t, federatedIssueGone(db.ErrNotFound))
	assert.False(t, federatedIssueGone(errors.New("scan failed")))
	assert.False(t, federatedIssueGone(nil))
}
