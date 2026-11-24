package daemon

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/db"
)

type claimPrincipal struct {
	db.ClaimPrincipal
	IdentityToken    bool
	OwnerAuthority   bool
	RequestPrincipal *Principal
}

const hostClaimClientKind = "host:v1"

func resolveClaimPrincipal(
	ctx context.Context,
	cfg ServerConfig,
	authz string,
	body api.ClaimActionBody,
) (context.Context, claimPrincipal, error) {
	return resolveCapabilityPrincipal(ctx, cfg, authz, body, true)
}

// resolveCapabilityPrincipal shares ordinary credential handling with claims.
// Only claim callers retain the historical keyless invalid-bearer fallback.
func resolveCapabilityPrincipal(
	ctx context.Context, cfg ServerConfig, authz string,
	body api.ClaimActionBody, keylessBearerFallback bool,
) (context.Context, claimPrincipal, error) {
	if cfg.HostAccess != nil {
		if requestPrincipal, ok := PrincipalFromContext(ctx); ok {
			if !validHostPrincipal(requestPrincipal) {
				return ctx, claimPrincipal{}, api.NewError(http.StatusUnauthorized, "auth_required",
					"valid host principal required", "", nil)
			}
			return ctx, hostClaimPrincipal(cfg, body, requestPrincipal.Subject), nil
		}
		return ctx, claimPrincipal{}, api.NewError(http.StatusUnauthorized, "auth_required",
			"host principal required", "", nil)
	}
	if requestPrincipal, ok := PrincipalFromContext(ctx); ok {
		if err := ensureAttributedWriteAllowed(ctx); err != nil {
			return ctx, claimPrincipal{}, err
		}
		principal := localClaimPrincipalWithHolder(cfg, body, actorFor(ctx, body.Holder))
		principal.IdentityToken = requestPrincipal.Kind == PrincipalDBToken
		principal.RequestPrincipal = &requestPrincipal
		principal.OwnerAuthority = projectOwnerAuthority(ctx)
		return claimRequestContext(ctx, principal), principal, nil
	}
	if cfg.Auth.Token != "" {
		if principal, ok, err := resolveLocalClaimPrincipal(ctx, cfg, authz, body, false); ok || err != nil {
			return claimRequestContext(ctx, principal), principal, err
		}
		return ctx, claimPrincipal{}, localAuthError(cfg, authz)
	}

	if hasBearerHeader(authz) && !keylessBearerFallback {
		return ctx, claimPrincipal{}, localAuthError(cfg, authz)
	}
	if cfg.InsecureReadonly {
		return ctx, claimPrincipal{}, localAuthError(cfg, authz)
	}
	principal := localClaimPrincipal(cfg, body)
	principal.OwnerAuthority = projectOwnerAuthority(ctx)
	return claimRequestContext(ctx, principal), principal, nil
}

func hostClaimPrincipal(cfg ServerConfig, body api.ClaimActionBody, subject string) claimPrincipal {
	principal := localClaimPrincipalWithHolder(cfg, body, hostClaimHolder(subject))
	principal.ClientKind = hostClaimClientKind
	principal.AuthenticatedHost = true
	return principal
}

func hostClaimHolder(subject string) string {
	digest := sha256.Sum256([]byte("poomsae:host-claim-holder:v1\x00" + subject))
	return "host:" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func authorizeClaimStatusRead(
	ctx context.Context,
	cfg ServerConfig,
	authz string,
) (context.Context, error) {
	if cfg.HostAccess != nil {
		if requestPrincipal, ok := PrincipalFromContext(ctx); ok {
			if validHostPrincipal(requestPrincipal) {
				return ctx, nil
			}
			return ctx, api.NewError(http.StatusUnauthorized, "auth_required",
				"valid host principal required", "", nil)
		}
		return ctx, api.NewError(http.StatusUnauthorized, "auth_required",
			"host principal required", "", nil)
	}
	if cfg.Auth.Token == "" {
		if hasBearerHeader(authz) {
			return ctx, localAuthError(cfg, authz)
		}
		if cfg.InsecureReadonly {
			return ctx, localAuthError(cfg, authz)
		}
		principal := localClaimPrincipal(cfg, api.ClaimActionBody{})
		principal.OwnerAuthority = projectOwnerAuthority(ctx)
		return claimRequestContext(ctx, principal), nil
	}
	if principal, ok, err := resolveLocalClaimPrincipal(ctx, cfg, authz, api.ClaimActionBody{}, true); ok || err != nil {
		return claimRequestContext(ctx, principal), err
	}
	return ctx, localAuthError(cfg, authz)
}

func resolveLocalClaimPrincipal(
	ctx context.Context,
	cfg ServerConfig,
	authz string,
	body api.ClaimActionBody,
	allowBootstrap bool,
) (claimPrincipal, bool, error) {
	switch {
	case cfg.Auth.Token == "":
		if cfg.InsecureReadonly {
			return claimPrincipal{}, false, nil
		}
		principal := localClaimPrincipal(cfg, body)
		principal.OwnerAuthority = projectOwnerAuthority(ctx)
		return principal, true, nil
	case validLocalBearer(cfg.Auth.Token, authz):
		if cfg.Auth.RequireTokenIdentity && !allowBootstrap {
			return claimPrincipal{}, false, api.NewError(http.StatusForbidden, "bootstrap_token_write_forbidden",
				"bootstrap token cannot perform attributed writes; use a user token", "", nil)
		}
		principal := localClaimPrincipal(cfg, body)
		principal.OwnerAuthority = true // configured bearer verified above
		return principal, true, nil
	case cfg.Auth.RequireTokenIdentity && hasBearerHeader(authz):
		if cfg.DB == nil {
			return claimPrincipal{}, false, api.NewError(http.StatusInternalServerError, "internal",
				"token identity requires a database", "", nil)
		}
		presented := strings.TrimPrefix(authz, authBearerPrefix)
		token, err := cfg.DB.ResolveAPIToken(ctx, presented)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return claimPrincipal{}, false, nil
			}
			return claimPrincipal{}, false, api.NewError(http.StatusInternalServerError, "internal",
				"token identity lookup failed", "", nil)
		}
		principal := localClaimPrincipalWithHolder(cfg, body, token.Actor)
		principal.IdentityToken = true
		nativePrincipal := principalFromAPIToken(token)
		principal.RequestPrincipal = &nativePrincipal
		return principal, true, nil
	default:
		return claimPrincipal{}, false, nil
	}
}

func claimRequestContext(ctx context.Context, principal claimPrincipal) context.Context {
	if principal.OwnerAuthority {
		// Only validated daemon credentials or the derived owner-local transport
		// grant this authority. Holder labels are attribution, never access.
		var limits []string
		if decision, _ := ctx.Value(projectAccessContextKey{}).(*ProjectAccessDecision); decision != nil {
			decision.owner = true
			limits = decision.fixedUIDs
			decision.ProjectUIDs = narrowProjectUIDs(nil, limits)
			decision.targets = nil
		}
		ctx = db.WithAuthorizedProjects(ctx, limits)
	}
	if principal.RequestPrincipal == nil {
		return ctx
	}
	return WithPrincipal(ctx, *principal.RequestPrincipal)
}

func localClaimPrincipal(cfg ServerConfig, body api.ClaimActionBody) claimPrincipal {
	return localClaimPrincipalWithHolder(cfg, body, body.Holder)
}

func localClaimPrincipalWithHolder(cfg ServerConfig, body api.ClaimActionBody, holder string) claimPrincipal {
	return claimPrincipal{
		HolderInstanceUID: cfg.DB.InstanceUID(),
		Holder:            strings.TrimSpace(holder),
		ClientKind:        strings.TrimSpace(body.ClientKind),
	}
}

func hasBearerHeader(authz string) bool {
	return strings.HasPrefix(authz, authBearerPrefix)
}

func validLocalBearer(token, authz string) bool {
	if token == "" || !strings.HasPrefix(authz, authBearerPrefix) {
		return false
	}
	presented := strings.TrimPrefix(authz, authBearerPrefix)
	return subtle.ConstantTimeCompare([]byte(presented), []byte(token)) == 1
}

func localAuthError(cfg ServerConfig, authz string) error {
	if cfg.Auth.Token == "" {
		return api.NewError(http.StatusUnauthorized, "auth_required",
			"mutations and event stream require authentication; daemon is in --insecure-readonly mode", "", nil)
	}
	if !strings.HasPrefix(authz, authBearerPrefix) {
		return api.NewError(http.StatusUnauthorized, "auth_required",
			"Authorization bearer required", "", nil)
	}
	return api.NewError(http.StatusForbidden, "auth_invalid", "token mismatch", "", nil)
}

func tokenScopeOut(scope *db.APITokenScope) *api.TokenScopeOut {
	if scope == nil {
		return nil
	}
	return &api.TokenScopeOut{
		Kind: string(scope.Kind), ProjectUID: scope.ProjectUID, RootIssueUID: scope.RootIssueUID,
	}
}
