package daemon

import (
	"errors"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/db"
)

// federationReadOnlyError maps storage refusals that still mention a federated
// project. This daemon no longer forwards those writes.
func federationReadOnlyError(err error) error {
	if errors.Is(err, db.ErrFederatedMoveUnsupported) {
		return api.NewError(409, "federated_move_unsupported",
			"cross-project moves involving a federated project are unsupported unless both projects use the same active hub",
			"create the issue in the target project and link it to the original issue", nil)
	}
	if errors.Is(err, db.ErrFederatedReadOnly) {
		if errors.Is(err, db.ErrRecurringHubRequired) {
			return api.NewError(409, "federated_read_only", db.ErrRecurringHubRequired.Error(),
				"Run the same command against the project's owning hub, for example with --daemon <hub-name>.", nil)
		}
		return api.NewError(409, "federated_read_only", err.Error(), "", nil)
	}
	return nil
}
