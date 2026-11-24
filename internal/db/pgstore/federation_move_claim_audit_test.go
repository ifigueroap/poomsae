package pgstore

import (
	"testing"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/stretchr/testify/require"
)

func TestMoveSelectsSubjectForClaimAudit(t *testing.T) {
	subject := "01HZNQ7VFPK1XGD8R5MABCD4EX"
	issues, err := federationIngestClaimAuditIssueUIDs(db.RemoteEvent{Type: "issue.moved", IssueUID: &subject})
	require.NoError(t, err)
	require.Equal(t, []federationIngestClaimAuditIssue{{UID: subject, RequireClaim: true}}, issues)
}
