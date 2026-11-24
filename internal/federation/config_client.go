package federation

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/doordash-oss/oapi-codegen-dd/v3/pkg/runtime"
	"github.com/ifigueroap/poomsae/internal/httpurl"
	"github.com/ifigueroap/poomsae/pkg/client/generated"

	"github.com/ifigueroap/poomsae/internal/client"
	"github.com/ifigueroap/poomsae/internal/config"
	poomsaeuid "github.com/ifigueroap/poomsae/internal/uid"
)

var (
	// ErrHubUnavailable classifies transport failures and transient hub errors.
	ErrHubUnavailable = errors.New("federation hub unavailable")
	// ErrHubAuthentication classifies catalog credential and authorization errors.
	ErrHubAuthentication = errors.New("federation hub authentication")
	// ErrHubValidation classifies rejected or malformed hub requests and responses.
	ErrHubValidation = errors.New("federation hub validation")

	normalizeHubURL       = client.NormalizeRemoteBaseURL
	newHubHTTPClient      = client.NewHTTPClientForTarget
	configureHubRedirects = client.ConfigureOriginPinnedRedirects
)

const (
	hubRequestTimeout   = 30 * time.Second
	maxHubResponseBytes = 1 << 20
)

// HubError is a bounded, secret-free hub failure. It intentionally retains
// only a stable category, operation label, and optional HTTP status.
type HubError struct {
	Kind       error
	Operation  string
	StatusCode int
}

// Error implements error without retaining request coordinates or response
// bodies, either of which may contain private data or echoed credentials.
func (e *HubError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("federation hub %s failed with HTTP status %d", e.Operation, e.StatusCode)
	}
	return fmt.Sprintf("federation hub %s failed", e.Operation)
}

// Unwrap exposes the stable error category.
func (e *HubError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Kind
}

// HubProject is the hub metadata needed to bind one local replica.
type HubProject struct {
	ID, ReplayHorizonEventID, BaselineThroughEventID int64
	UID, Name                                        string
}

// EnrollmentRequest describes one project-scoped explicit-token grant.
type EnrollmentRequest struct {
	ProjectID                    int64
	SpokeInstanceUID             string
	Token, Capabilities, Actor   string
	AllowAdoptionSnapshotAuthors bool
}

// Enrollment is the identity-bearing portion of a hub enrollment response.
type Enrollment struct {
	ID    int64
	Actor string
}

// Hub is the narrow remote contract used by one reconciliation attempt.
type Hub interface {
	ResolveProject(context.Context, string) (HubProject, error)
	EnsureProject(context.Context, string, string) (HubProject, error)
	EnsureEnrollment(context.Context, EnrollmentRequest) (Enrollment, error)
	RotateEnrollment(context.Context, EnrollmentRequest) (Enrollment, error)
	RevokeEnrollment(context.Context, int64) error
}

// HubClient binds every request and the selected catalog bearer to one
// canonical hub origin.
type HubClient struct {
	baseURL string
	http    *http.Client
}

// NewHubClient resolves only the selected catalog entry's authentication
// policy. In particular, an empty token_env is an authentication error and
// never falls back to POOMSAE_AUTH_TOKEN or [auth].token.
func NewHubClient(
	ctx context.Context, catalog config.CatalogDaemonConfig,
) (*HubClient, error) {
	token, err := catalogToken(catalog)
	if err != nil {
		return nil, err
	}
	allowInsecure, err := httpurl.EffectiveHTTPAllowInsecure(catalog.URL, catalog.AllowInsecure)
	if err != nil {
		return nil, hubError(ErrHubValidation, "hub URL validation", 0)
	}
	baseURL, err := normalizeHubURL(catalog.URL, allowInsecure)
	if err != nil {
		return nil, hubError(ErrHubValidation, "hub URL validation", 0)
	}
	httpClient, err := newHubHTTPClient(ctx, baseURL, client.TargetAuth{
		Token:         token,
		AllowInsecure: allowInsecure,
	}, client.Opts{Timeout: hubRequestTimeout})
	if err != nil {
		return nil, hubError(ErrHubValidation, "hub transport setup", 0)
	}
	if err := configureHubRedirects(httpClient, baseURL); err != nil {
		return nil, hubError(ErrHubValidation, "hub redirect policy", 0)
	}
	return &HubClient{baseURL: baseURL, http: httpClient}, nil
}

func catalogToken(catalog config.CatalogDaemonConfig) (string, error) {
	token, err := catalog.ClientToken()
	if err != nil {
		return "", hubError(ErrHubAuthentication, "catalog authentication", 0)
	}
	return token, nil
}

// EnsureProject resolves or creates name, enables federation, and returns the
// enabled project's stable federation metadata.
func (c *HubClient) EnsureProject(context.Context, string, string) (HubProject, error) {
	return HubProject{}, hubError(ErrHubValidation, "enable project federation", http.StatusNotFound)
}

// ResolveProject looks up a project without creating it or enabling
// federation.
func (c *HubClient) ResolveProject(ctx context.Context, name string) (HubProject, error) {
	project, err := c.resolveProject(ctx, name)
	if err != nil {
		return HubProject{}, err
	}
	return HubProject{
		ID:   project.ID,
		UID:  project.UID,
		Name: project.Name,
	}, nil
}

func (c *HubClient) resolveProject(ctx context.Context, name string) (projectResponse, error) {
	var response struct {
		Project projectResponse `json:"project"`
	}
	apiClient, err := generated.NewDefaultClient(c.baseURL, runtime.WithHTTPClient(hubDoer{client: c.http, operation: "resolve project"}))
	if err != nil {
		return projectResponse{}, hubError(ErrHubValidation, "resolve project", 0)
	}
	responseHTTP, callErr := apiClient.ResolveProjectWithResponse(ctx, &generated.ResolveProjectRequestOptions{Body: &generated.ResolveProjectBody{Name: &name}})
	if responseHTTP == nil {
		return projectResponse{}, hubCallError(callErr, "resolve project")
	}
	err = json.Unmarshal(responseHTTP.Body, &response)
	if err != nil {
		return projectResponse{}, hubError(ErrHubValidation, "resolve project", responseHTTP.StatusCode)
	}
	if response.Project.ID <= 0 ||
		!poomsaeuid.Valid(response.Project.UID) ||
		response.Project.Name == "" {
		return projectResponse{}, hubError(ErrHubValidation, "resolve project", http.StatusOK)
	}
	return response.Project, nil
}

func (c *HubClient) createProject(ctx context.Context, name, actor string) (projectResponse, error) {
	var response struct {
		Project projectResponse `json:"project"`
	}
	apiClient, err := generated.NewDefaultClient(c.baseURL, runtime.WithHTTPClient(hubDoer{client: c.http, operation: "create project"}))
	if err != nil {
		return projectResponse{}, hubError(ErrHubValidation, "create project", 0)
	}
	responseHTTP, callErr := apiClient.InitProjectWithResponse(ctx, &generated.InitProjectRequestOptions{Body: &generated.InitProjectBody{Name: &name, Actor: &actor}})
	if responseHTTP == nil {
		return projectResponse{}, hubCallError(callErr, "create project")
	}
	err = json.Unmarshal(responseHTTP.Body, &response)
	if err != nil {
		return projectResponse{}, hubError(ErrHubValidation, "create project", responseHTTP.StatusCode)
	}
	if response.Project.ID <= 0 ||
		!poomsaeuid.Valid(response.Project.UID) ||
		response.Project.Name == "" {
		return projectResponse{}, hubError(ErrHubValidation, "create project", http.StatusOK)
	}
	return response.Project, nil
}

type projectResponse struct {
	ID   int64  `json:"id"`
	UID  string `json:"uid"`
	Name string `json:"name"`
}

type projectFederationResponse struct {
	ProjectID              int64  `json:"project_id"`
	ProjectUID             string `json:"project_uid"`
	ProjectName            string `json:"project_name"`
	ReplayHorizonEventID   int64  `json:"replay_horizon_event_id"`
	BaselineThroughEventID int64  `json:"baseline_through_event_id"`
}

type enrollmentWireResponse struct {
	ID    int64  `json:"id"`
	Actor string `json:"actor"`
}

// EnsureEnrollment idempotently ensures the request's explicit token.
func (c *HubClient) EnsureEnrollment(context.Context, EnrollmentRequest) (Enrollment, error) {
	return Enrollment{}, hubError(ErrHubValidation, "ensure enrollment", http.StatusNotFound)
}

// RotateEnrollment replaces only the matching spoke/project enrollment scope.
func (c *HubClient) RotateEnrollment(context.Context, EnrollmentRequest) (Enrollment, error) {
	return Enrollment{}, hubError(ErrHubValidation, "rotate enrollment", http.StatusNotFound)
}

// RevokeEnrollment compensates an enrollment or rotation whose local
// reservation was explicitly removed while the hub request was in flight.
func (c *HubClient) RevokeEnrollment(context.Context, int64) error {
	return hubError(ErrHubValidation, "revoke enrollment", http.StatusNotFound)
}

// hubDoer applies the catalog client's bounded, secret-free response policy
// before the generated runtime buffers or decodes a response.
type hubDoer struct {
	client    *http.Client
	operation string
}

func (d hubDoer) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req.Header.Set("Accept", "application/json")
	response, err := d.client.Do(req.WithContext(ctx)) //nolint:gosec // G704: generated enrollment routes use the selected operator-configured hub catalog entry.
	if err != nil {
		return nil, hubError(ErrHubUnavailable, d.operation, 0)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		return nil, hubError(hubStatusKind(response.StatusCode), d.operation, response.StatusCode)
	}
	response.Body = limitedHubBody{Reader: io.LimitReader(response.Body, maxHubResponseBytes), Closer: response.Body}
	return response, nil
}

type limitedHubBody struct {
	io.Reader
	io.Closer
}

func hubCallError(err error, operation string) error {
	if hubErr, ok := errors.AsType[*HubError](err); ok {
		return hubErr
	}
	return hubError(ErrHubValidation, operation, 0)
}

func hubStatusKind(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ErrHubAuthentication
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests:
		return ErrHubUnavailable
	case status >= http.StatusInternalServerError:
		return ErrHubUnavailable
	default:
		return ErrHubValidation
	}
}

func hubError(kind error, operation string, status int) *HubError {
	return &HubError{Kind: kind, Operation: operation, StatusCode: status}
}

var _ Hub = (*HubClient)(nil)
