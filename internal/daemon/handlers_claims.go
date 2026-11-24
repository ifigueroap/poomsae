package daemon

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/db"
)

// projectScopedClaimActionEvent rewrites the lease action response's event
// through the typed issue-scoped projection. Unscoped callers are untouched.
func projectScopedClaimActionEvent(
	ctx context.Context, store db.Storage, body *api.ClaimActionResponseBody,
) error {
	if issueScopeFromContext(ctx) == nil || body == nil || body.Event == nil {
		return nil
	}
	projected, err := scopedMutationEvent(ctx, store, body.Event)
	if err != nil {
		return err
	}
	body.Event = projected
	return nil
}

func registerClaimHandlers(humaAPI huma.API, cfg ServerConfig) {
	huma.Register(humaAPI, huma.Operation{
		OperationID: "acquireIssueLease",
		Method:      http.MethodPost,
		Path:        "/api/v1/projects/{project_id}/issues/{ref}/lease/actions/acquire",
	}, func(ctx context.Context, in *api.ClaimActionRequest) (*api.ClaimActionResponse, error) {
		ctx, principal, err := resolveClaimPrincipal(ctx, cfg, in.Authorization, in.Body)
		if err != nil {
			return nil, err
		}
		issue, err := locateClaimIssue(ctx, cfg.DB, in.ProjectID, in.Ref)
		if err != nil {
			return nil, err
		}
		if issue.ProjectID != in.ProjectID {
			ctx, principal, err = resolveClaimPrincipal(ctx, cfg, in.Authorization, in.Body)
			if err != nil {
				return nil, err
			}
		}
		ctx, err = authorizeHostProjectScope(ctx, []int64{issue.ProjectID}, nil, false)
		if err != nil {
			return nil, err
		}
		issue, err = activeIssueByRef(ctx, cfg.DB, issue.ProjectID, issue.UID, db.IncludeDeletedNo)
		if err != nil {
			return nil, err
		}
		body, err := handleClaimAcquire(ctx, cfg, issue.ProjectID, issue.UID, in.Body, principal.ClaimPrincipal)
		if err != nil {
			return nil, err
		}
		if err := projectScopedClaimActionEvent(ctx, cfg.DB, &body); err != nil {
			return nil, err
		}
		return &api.ClaimActionResponse{Body: body}, nil
	})

	huma.Register(humaAPI, huma.Operation{
		OperationID: "renewIssueLease",
		Method:      http.MethodPost,
		Path:        "/api/v1/projects/{project_id}/issues/{ref}/lease/actions/renew",
	}, func(ctx context.Context, in *api.ClaimActionRequest) (*api.ClaimActionResponse, error) {
		ctx, principal, err := resolveClaimPrincipal(ctx, cfg, in.Authorization, in.Body)
		if err != nil {
			return nil, err
		}
		issue, err := locateClaimIssue(ctx, cfg.DB, in.ProjectID, in.Ref)
		if err != nil {
			return nil, err
		}
		if issue.ProjectID != in.ProjectID {
			ctx, principal, err = resolveClaimPrincipal(ctx, cfg, in.Authorization, in.Body)
			if err != nil {
				return nil, err
			}
		}
		ctx, err = authorizeHostProjectScope(ctx, []int64{issue.ProjectID}, nil, false)
		if err != nil {
			return nil, err
		}
		issue, err = activeIssueByRef(ctx, cfg.DB, issue.ProjectID, issue.UID, db.IncludeDeletedNo)
		if err != nil {
			return nil, err
		}
		body, err := handleClaimRenew(ctx, cfg, issue.ProjectID, issue.UID, in.Body, principal.ClaimPrincipal)
		if err != nil {
			return nil, err
		}
		if err := projectScopedClaimActionEvent(ctx, cfg.DB, &body); err != nil {
			return nil, err
		}
		return &api.ClaimActionResponse{Body: body}, nil
	})

	huma.Register(humaAPI, huma.Operation{
		OperationID: "releaseIssueLease",
		Method:      http.MethodPost,
		Path:        "/api/v1/projects/{project_id}/issues/{ref}/lease/actions/release",
	}, func(ctx context.Context, in *api.ClaimActionRequest) (*api.ClaimActionResponse, error) {
		ctx, principal, err := resolveClaimPrincipal(ctx, cfg, in.Authorization, in.Body)
		if err != nil {
			return nil, err
		}
		issue, err := locateClaimIssue(ctx, cfg.DB, in.ProjectID, in.Ref)
		if err != nil {
			return nil, err
		}
		if issue.ProjectID != in.ProjectID {
			ctx, principal, err = resolveClaimPrincipal(ctx, cfg, in.Authorization, in.Body)
			if err != nil {
				return nil, err
			}
		}
		ctx, err = authorizeHostProjectScope(ctx, []int64{issue.ProjectID}, nil, false)
		if err != nil {
			return nil, err
		}
		issue, err = activeIssueByRef(ctx, cfg.DB, issue.ProjectID, issue.UID, db.IncludeDeletedNo)
		if err != nil {
			return nil, err
		}
		body, err := handleClaimRelease(ctx, cfg, issue.ProjectID, issue.UID, in.Body, principal.ClaimPrincipal)
		if err != nil {
			return nil, err
		}
		if err := projectScopedClaimActionEvent(ctx, cfg.DB, &body); err != nil {
			return nil, err
		}
		return &api.ClaimActionResponse{Body: body}, nil
	})

	huma.Register(humaAPI, huma.Operation{
		OperationID: "forceReleaseIssueLease",
		Method:      http.MethodPost,
		Path:        "/api/v1/projects/{project_id}/issues/{ref}/lease/actions/force_release",
	}, func(ctx context.Context, in *api.ClaimActionRequest) (*api.ClaimActionResponse, error) {
		ctx, principal, err := resolveClaimPrincipal(ctx, cfg, in.Authorization, in.Body)
		if err != nil {
			return nil, err
		}
		if issueScopeFromContext(ctx) != nil {
			return nil, api.NewError(http.StatusForbidden, "scoped_operation_forbidden",
				"operation is not available to an issue-scoped credential", "", nil)
		}
		issue, err := locateClaimIssueWithDeleted(ctx, cfg.DB, in.ProjectID, in.Ref, db.IncludeDeletedYes)
		if err != nil {
			return nil, err
		}
		if issue.ProjectID != in.ProjectID {
			ctx, principal, err = resolveClaimPrincipal(ctx, cfg, in.Authorization, in.Body)
			if err != nil {
				return nil, err
			}
		}
		ctx, err = authorizeHostProjectScope(ctx, []int64{issue.ProjectID}, nil, false)
		if err != nil {
			return nil, err
		}
		issue, err = activeIssueByRef(ctx, cfg.DB, issue.ProjectID, issue.UID, db.IncludeDeletedYes)
		if err != nil {
			return nil, err
		}
		actor := actorFor(ctx, strings.TrimSpace(in.Body.Actor))
		if principal.IdentityToken {
			actor = principal.Holder
		}
		if actor == "" {
			actor = "admin"
		}
		reason := strings.TrimSpace(in.Body.Reason)
		if reason == "" {
			reason = "admin_force_release"
		}
		result, err := cfg.DB.ForceReleaseClaim(ctx, db.ForceReleaseClaimParams{
			ProjectID: issue.ProjectID,
			IssueRef:  issue.UID,
			Actor:     actor,
			Reason:    reason,
			Now:       time.Now().UTC(),
		})
		if err != nil {
			if errors.Is(err, db.ErrClaimExpired) {
				cfg.Publish().EventsByProject(result.Events)
			}
			return nil, claimAPIError(err)
		}
		cfg.Publish().EventsByProject(result.Events)
		if result.Event != nil {
			cfg.Publish().Event(result.Event.ProjectID, *result.Event)
		}
		return &api.ClaimActionResponse{Body: claimResultBody(result)}, nil
	})

	huma.Register(humaAPI, huma.Operation{
		OperationID: "getIssueLeaseStatus",
		Method:      http.MethodGet,
		Path:        "/api/v1/projects/{project_id}/issues/{ref}/lease",
	}, func(ctx context.Context, in *api.ClaimStatusRequest) (*api.ClaimStatusResponse, error) {
		var err error
		ctx, err = authorizeClaimStatusRead(ctx, cfg, in.Authorization)
		if err != nil {
			return nil, err
		}
		issue, err := locateClaimIssue(ctx, cfg.DB, in.ProjectID, in.Ref)
		if err != nil {
			return nil, err
		}
		if issue.ProjectID != in.ProjectID {
			ctx, err = authorizeClaimStatusRead(ctx, cfg, in.Authorization)
			if err != nil {
				return nil, err
			}
		}
		ctx, err = authorizeHostProjectScope(ctx, []int64{issue.ProjectID}, nil, false)
		if err != nil {
			return nil, err
		}
		issue, err = activeIssueByRef(ctx, cfg.DB, issue.ProjectID, issue.UID, db.IncludeDeletedNo)
		if err != nil {
			return nil, err
		}
		body, err := handleClaimStatus(ctx, cfg, issue.ProjectID, issue.UID)
		if err != nil {
			return nil, err
		}
		return &api.ClaimStatusResponse{Body: body}, nil
	})
}

// locateClaimIssue selects routing identity before the current project's claim
// credential and host scope are checked. Enrollment authorization restricts DB
// reads to the route project even when the enrollment permits both projects.
// Callers must reauthorize and use activeIssueByRef before accessing lease state.
func locateClaimIssue(ctx context.Context, store db.Storage, projectID int64, ref string) (db.Issue, error) {
	return locateClaimIssueWithDeleted(ctx, store, projectID, ref, db.IncludeDeletedNo)
}

func locateClaimIssueWithDeleted(ctx context.Context, store db.Storage, projectID int64, ref string, include db.IncludeDeleted) (db.Issue, error) {
	if _, err := activeProjectByID(ctx, store, projectID); err != nil {
		return db.Issue{}, err
	}
	return resolveIssueRef(db.WithAuthorizedProjects(ctx, nil), store, projectID, ref, include)
}

func localClaimProject(ctx context.Context, store db.Storage, projectID int64) error {
	_, err := activeProjectByID(ctx, store, projectID)
	return err
}

func handleClaimAcquire(
	ctx context.Context,
	cfg ServerConfig,
	projectID int64,
	ref string,
	body api.ClaimActionBody,
	principal db.ClaimPrincipal,
) (api.ClaimActionResponseBody, error) {
	if err := localClaimProject(ctx, cfg.DB, projectID); err != nil {
		return api.ClaimActionResponseBody{}, err
	}
	result, err := cfg.DB.AcquireClaim(ctx, db.AcquireClaimParams{
		ProjectID: projectID,
		IssueRef:  ref,
		Principal: principal,
		ClaimKind: claimKindOrDefault(body.ClaimKind),
		TTL:       ttlDuration(body.TTLSeconds),
		Purpose:   strings.TrimSpace(body.Purpose),
		Now:       time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, db.ErrClaimDenied) {
			return claimResultBody(result), nil
		}
		return api.ClaimActionResponseBody{}, claimAPIError(err)
	}
	cfg.Publish().EventsByProject(result.Events)
	if result.Event != nil {
		cfg.Publish().Event(projectID, *result.Event)
	}
	return claimResultBody(result), nil
}

func handleClaimRenew(
	ctx context.Context,
	cfg ServerConfig,
	projectID int64,
	ref string,
	body api.ClaimActionBody,
	principal db.ClaimPrincipal,
) (api.ClaimActionResponseBody, error) {
	if err := localClaimProject(ctx, cfg.DB, projectID); err != nil {
		return api.ClaimActionResponseBody{}, err
	}
	result, err := cfg.DB.RenewClaim(ctx, db.RenewClaimParams{
		ProjectID: projectID,
		IssueRef:  ref,
		Principal: principal,
		TTL:       ttlDuration(body.TTLSeconds),
		Now:       time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, db.ErrClaimExpired) {
			cfg.Publish().EventsByProject(result.Events)
		}
		return api.ClaimActionResponseBody{}, claimAPIError(err)
	}
	cfg.Publish().EventsByProject(result.Events)
	return claimResultBody(result), nil
}

func handleClaimRelease(
	ctx context.Context,
	cfg ServerConfig,
	projectID int64,
	ref string,
	body api.ClaimActionBody,
	principal db.ClaimPrincipal,
) (api.ClaimActionResponseBody, error) {
	if err := localClaimProject(ctx, cfg.DB, projectID); err != nil {
		return api.ClaimActionResponseBody{}, err
	}
	result, err := cfg.DB.ReleaseClaim(ctx, db.ReleaseClaimParams{
		ProjectID: projectID,
		IssueRef:  ref,
		Principal: principal,
		Reason:    strings.TrimSpace(body.Reason),
		Now:       time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, db.ErrClaimExpired) {
			cfg.Publish().EventsByProject(result.Events)
		}
		return api.ClaimActionResponseBody{}, claimAPIError(err)
	}
	cfg.Publish().EventsByProject(result.Events)
	if result.Event != nil {
		cfg.Publish().Event(projectID, *result.Event)
	}
	return claimResultBody(result), nil
}

func handleClaimStatus(ctx context.Context, cfg ServerConfig, projectID int64, ref string) (api.ClaimStatusBody, error) {
	if err := localClaimProject(ctx, cfg.DB, projectID); err != nil {
		return api.ClaimStatusBody{}, err
	}
	status, err := cfg.DB.ClaimStatus(ctx, projectID, ref, time.Now().UTC())
	if err != nil {
		return api.ClaimStatusBody{}, claimAPIError(err)
	}
	cfg.Publish().EventsByProject(status.Events)
	return claimStatusBody(status), nil
}

// nonNilEvents upholds the claim response contract that events is a JSON array.
func nonNilEvents(events []db.Event) []db.Event {
	if events == nil {
		return []db.Event{}
	}
	return events
}

func claimKindOrDefault(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return "hard"
	}
	return kind
}

func ttlDuration(seconds int64) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func claimAPIError(err error) error {
	switch {
	case errors.Is(err, db.ErrTransactionFinalizationFailed):
		return internalAPIError(err)
	case errors.Is(err, ErrHostAccessDenied):
		return api.NewError(http.StatusForbidden, "auth_invalid", "host access denied", "", nil)
	case errors.Is(err, db.ErrClaimDenied):
		return api.NewError(http.StatusConflict, "claim_denied", err.Error(), "", nil)
	case errors.Is(err, db.ErrClaimRequired):
		return api.NewError(http.StatusConflict, "claim_required", err.Error(), "", nil)
	case errors.Is(err, db.ErrClaimNotHeld):
		return api.NewError(http.StatusConflict, "claim_not_held", err.Error(), "", nil)
	case errors.Is(err, db.ErrClaimExpired):
		return api.NewError(http.StatusConflict, "claim_expired", err.Error(), "", nil)
	case errors.Is(err, db.ErrClaimValidation):
		return api.NewError(http.StatusBadRequest, "validation", err.Error(), "", nil)
	case errors.Is(err, db.ErrNotFound):
		return api.NewError(http.StatusNotFound, "issue_not_found", "issue not found", "", nil)
	default:
		return internalAPIError(err)
	}
}

func claimResultBody(result db.LeaseResult) api.ClaimActionResponseBody {
	body := api.ClaimActionResponseBody{
		Granted: result.Granted,
		Holder:  claimPrincipalOut(result.Holder),
		Lease:   issueClaimOut(result.Claim),
		Event:   result.Event,
	}
	body.MirrorDeprecatedClaimFields()
	return body
}

func claimStatusBody(status db.ClaimStatus) api.ClaimStatusBody {
	body := api.ClaimStatusBody{
		Held:   status.Held,
		Holder: claimPrincipalOut(status.Holder),
		Lease:  issueClaimOut(status.Claim),
		HubNow: status.HubNow,
	}
	body.MirrorDeprecatedClaimFields()
	return body
}

const showClaimStatusRetryAfter = time.Minute

// showClaimStatusRefreshTimeout is a var so tests can exercise the budget without waiting it out.
var showClaimStatusRefreshTimeout = 500 * time.Millisecond

func showIssueClaimRelevant(ctx context.Context, store db.Storage, projectID int64) (bool, error) {
	return true, nil
}

func refreshShowClaimStatus(ctx context.Context, cfg ServerConfig, issue db.Issue) (*time.Time, error) {
	status, err := cfg.DB.ClaimStatusReadOnly(ctx, issue.ProjectID, issue.UID, time.Now().UTC())
	if err != nil {
		return nil, claimAPIError(err)
	}
	if status.HubNow.IsZero() {
		return nil, nil
	}
	hubNow := status.HubNow
	return &hubNow, nil
}

func hydrateClaimOutForIssue(ctx context.Context, cfg ServerConfig, issue db.Issue, out *api.ShowIssueResponse) error {
	now := time.Now().UTC()
	status, err := cfg.DB.ClaimStatusReadOnly(ctx, issue.ProjectID, issue.UID, now)
	if err != nil {
		return internalAPIError(err)
	}
	pending, err := cfg.DB.ListPendingClaimRequestsForIssue(ctx, issue.ProjectID, issue.UID, 0)
	if err != nil {
		return internalAPIError(err)
	}
	out.Body.Lease = issueClaimOut(status.Claim)
	if len(pending) > 0 {
		out.Body.PendingLeases = pendingClaimOuts(pending)
	}
	if out.Body.Lease != nil || len(pending) > 0 {
		hubNow := status.HubNow
		if hubNow.IsZero() {
			hubNow = now
		}
		out.Body.LeaseHubNow = &hubNow
	}
	return nil
}

func pendingClaimOuts(pending []db.PendingClaimRequest) []api.PendingClaimOut {
	out := make([]api.PendingClaimOut, 0, len(pending))
	for _, req := range pending {
		out = append(out, api.PendingClaimOut{
			RequestUID:        req.RequestUID,
			Holder:            req.Holder,
			HolderInstanceUID: req.HolderInstanceUID,
			ClientKind:        req.ClientKind,
			ClaimKind:         req.ClaimKind,
			TTLSeconds:        req.TTLSeconds,
			Purpose:           req.Purpose,
			RequestedAt:       req.RequestedAt,
			LastAttemptAt:     req.LastAttemptAt,
			LastError:         req.LastError,
		})
	}
	return out
}

func claimPrincipalOut(p db.ClaimPrincipal) api.ClaimPrincipalOut {
	return api.ClaimPrincipalOut{
		HolderInstanceUID: p.HolderInstanceUID,
		Holder:            p.Holder,
		ClientKind:        p.ClientKind,
	}
}

func issueClaimOut(claim *db.IssueClaim) *api.IssueClaimOut {
	if claim == nil {
		return nil
	}
	return &api.IssueClaimOut{
		ClaimUID:          claim.ClaimUID,
		ProjectID:         claim.ProjectID,
		IssueUID:          claim.IssueUID,
		Holder:            claim.Holder,
		HolderInstanceUID: claim.HolderInstanceUID,
		ClientKind:        claim.ClientKind,
		Purpose:           claim.Purpose,
		ClaimKind:         claim.ClaimKind,
		AcquiredAt:        claim.AcquiredAt,
		ExpiresAt:         claim.ExpiresAt,
		ReleasedAt:        claim.ReleasedAt,
		ReleaseReason:     claim.ReleaseReason,
		Revision:          claim.Revision,
		UpdatedAt:         claim.UpdatedAt,
	}
}
