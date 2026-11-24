package pgstore

import (
	"context"
	"iter"

	"github.com/ifigueroap/poomsae/internal/db"
)

// ExportMCPEventSubscriptions streams subscriptions for a full backup.
func (s *Store) ExportMCPEventSubscriptions(ctx context.Context, filter db.ExportFilter) iter.Seq2[db.MCPEventSubscriptionExport, error] {
	return db.ExportMCPEventSubscriptionsSQL(ctx, s.exportQueryContext, filter)
}
