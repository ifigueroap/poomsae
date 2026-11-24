package daemon

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/db"
)

// Leave the local lease check inside the CLI request budget.
const mutationClaimStatusRefreshTimeout = 500 * time.Millisecond

func requireFederatedIssueClaim(
	ctx context.Context,
	cfg ServerConfig,
	_ int64,
	issue db.Issue,
	actor string,
) error {
	return requireFederatedIssueClaimWithRefresh(ctx, cfg, issue, actor, true)
}

// Move previews check cached authority without remote refresh or cache writes.
func requireFederatedIssueClaimWithRefresh(
	ctx context.Context,
	cfg ServerConfig,
	issue db.Issue,
	actor string,
	refresh bool,
) error {
	return requireFederatedIssueClaimWithResult(ctx, cfg, issue, actor, refresh, nil)
}

type claimRefreshResult struct {
	binding db.FederationBinding
	known   bool
	missing bool
}

func requireFederatedIssueClaimWithResult(ctx context.Context, cfg ServerConfig, issue db.Issue, actor string, refresh bool, result *claimRefreshResult) error {
	principal := db.ClaimPrincipal{
		HolderInstanceUID: cfg.DB.InstanceUID(),
		Holder:            strings.TrimSpace(actor),
	}
	if cfg.HostAccess != nil {
		requestPrincipal, ok := PrincipalFromContext(ctx)
		if !ok || !validHostPrincipal(requestPrincipal) {
			return api.NewError(http.StatusUnauthorized, "auth_required",
				"valid host principal required", "", nil)
		}
		principal = hostClaimPrincipal(cfg, api.ClaimActionBody{}, requestPrincipal.Subject).ClaimPrincipal
	}
	return checkFederatedIssueClaim(ctx, cfg, issue, principal, refresh, false, result)
}

func requireFederatedIssueClaimForPrincipal(
	ctx context.Context,
	cfg ServerConfig,
	_ int64,
	issue db.Issue,
	principal db.ClaimPrincipal,
) error {
	return requireFederatedIssueClaimForPrincipalWithRefresh(ctx, cfg, issue, principal, true)
}

func requireFederatedIssueClaimForPrincipalWithRefresh(
	ctx context.Context,
	cfg ServerConfig,
	issue db.Issue,
	principal db.ClaimPrincipal,
	refresh bool,
) error {
	return checkFederatedIssueClaim(ctx, cfg, issue, principal, refresh, false, nil)
}

// checkFederatedIssueClaim enforces a local lease only for an enabled hub
// binding still stored on this daemon. Spoke bindings are not forwarded.
func checkFederatedIssueClaim(ctx context.Context, cfg ServerConfig, issue db.Issue, principal db.ClaimPrincipal, _, _ bool, _ *claimRefreshResult) error {
	projectID := issue.ProjectID
	binding, err := cfg.DB.FederationBindingByProject(ctx, projectID)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return internalAPIError(err)
	}
	if !binding.Enabled || binding.Role != db.FederationRoleHub {
		return nil
	}
	err = cfg.DB.CheckClaimGate(ctx, db.ClaimGateParams{
		ProjectID: projectID,
		IssueRef:  issue.UID,
		Principal: principal,
		Now:       time.Now().UTC(),
	})
	if err != nil {
		return claimGateAPIError(err)
	}
	return nil
}

func requireFederatedHubIssueClaim(
	ctx context.Context,
	cfg ServerConfig,
	_ int64,
	issue db.Issue,
	actor string,
) error {
	projectID := issue.ProjectID
	binding, err := cfg.DB.FederationBindingByProject(ctx, projectID)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return internalAPIError(err)
	}
	if !binding.Enabled || binding.Role != db.FederationRoleHub {
		return nil
	}
	return requireFederatedIssueClaim(ctx, cfg, projectID, issue, actor)
}

func claimGateAPIError(err error) error {
	switch {
	case errors.Is(err, db.ErrTransactionFinalizationFailed):
		return api.NewError(http.StatusServiceUnavailable, "access_unavailable", "access decision unavailable", "", nil)
	case errors.Is(err, ErrHostAccessDenied):
		return api.NewError(http.StatusForbidden, "auth_invalid", "host access denied", "", nil)
	case errors.Is(err, db.ErrClaimDenied):
		return api.NewError(http.StatusConflict, "claim_denied",
			"lease denied for federated issue mutation", "run poomsae federation lease acquire <ref>", nil)
	case errors.Is(err, db.ErrClaimExpired):
		return api.NewError(http.StatusConflict, "claim_expired",
			"lease expired for federated issue mutation", "run poomsae federation lease acquire <ref>", nil)
	case errors.Is(err, db.ErrClaimRequired), errors.Is(err, db.ErrPendingClaimNotAuthoritative):
		return api.NewError(http.StatusConflict, "claim_required",
			"lease is not authoritative for this federated issue mutation", "run poomsae show <ref>", nil)
	case errors.Is(err, db.ErrClaimValidation):
		return api.NewError(http.StatusBadRequest, "validation", err.Error(), "", nil)
	case errors.Is(err, db.ErrNotFound):
		return api.NewError(http.StatusNotFound, "issue_not_found", "issue not found", "", nil)
	default:
		return internalAPIError(err)
	}
}
