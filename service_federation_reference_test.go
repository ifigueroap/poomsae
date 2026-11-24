package poomsae

import (
	"context"
	"sync/atomic"
)

type projectPushController struct{}

func (projectPushController) AuthorizeFederation(context.Context, FederationAccessRequest) (FederationAccessDecision, error) {
	return FederationAccessDecision{TransactionFence: func(context.Context, Transaction) error { return nil }}, nil
}

type projectReferenceController struct {
	projectPushController
	allow atomic.Bool
}

func (c *projectReferenceController) AuthorizeFederationReference(_ context.Context, _ Transaction, route, peer FederationAccessRequest) error {
	if route.Project.UID == peer.Project.UID || route.Enrollment.SpokeInstanceUID != peer.Enrollment.SpokeInstanceUID ||
		route.Enrollment.Actor != "worker" || peer.Enrollment.Actor != "peer-member" || !c.allow.Load() {
		return ErrAccessDenied
	}
	return nil
}
