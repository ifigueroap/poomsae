package poomsae_test

import (
	"context"
	"slices"

	"github.com/ifigueroap/poomsae"
)

type spokeNameScopeAccess struct {
	projectID         int64
	allowGlobal       bool
	rejectGlobalFence bool
	globalRequests    int
	globalFences      int
}

func (a *spokeNameScopeAccess) Authorize(_ context.Context, request poomsae.AccessRequest) (poomsae.AccessDecision, error) {
	if request.Operation.ID != "createFederationEnrollment" {
		return poomsae.AccessDecision{}, poomsae.ErrAccessDenied
	}
	global := request.Operation.AllProjects
	if global {
		a.globalRequests++
		if !a.allowGlobal {
			return poomsae.AccessDecision{}, poomsae.ErrAccessDenied
		}
	} else if !slices.Equal(request.Operation.ProjectIDs, []int64{a.projectID}) {
		return poomsae.AccessDecision{}, poomsae.ErrAccessDenied
	}
	return poomsae.AccessDecision{TransactionFence: func(context.Context, poomsae.Transaction) error {
		if global {
			a.globalFences++
			if a.rejectGlobalFence {
				return poomsae.ErrAccessDenied
			}
		}
		return nil
	}}, nil
}
