package federation

import (
	"context"
	"crypto/sha256"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ifigueroap/poomsae/internal/federationsigning"
	"github.com/ifigueroap/poomsae/internal/httpurl"

	"github.com/ifigueroap/poomsae/internal/api"
	clientpkg "github.com/ifigueroap/poomsae/internal/client"
	"github.com/ifigueroap/poomsae/internal/db"
)

var errRelayCrossProjectLinks = errors.New("hub relay cross-project links refusal")

// HubStatusError reports a non-2xx response from the configured hub.
type HubStatusError struct {
	relayCrossProjectLinks bool
	Path                   string
	StatusCode             int
	Body                   string
	// RetryAfter is the hub's Retry-After hint, or zero when it sent none.
	RetryAfter time.Duration
}

// Is classifies the canonical refusal even through joined sync errors.
func (e *HubStatusError) Is(target error) bool {
	return target == errRelayCrossProjectLinks && e.relayCrossProjectLinks
}

// HTTPStatus exposes the hub response code to outcome classification.
func (e *HubStatusError) HTTPStatus() int { return e.StatusCode }

func (e *HubStatusError) Error() string {
	return fmt.Sprintf("hub %s returned %d: %s", e.Path, e.StatusCode, e.Body)
}

// Client is the outbound hub client used by pull replication.
type Client struct {
	baseURL string
	client  *http.Client
	// tokenDigest identifies the bearer without keeping another copy of it.
	tokenDigest [sha256.Size]byte
}

// NewClient builds a bearer-pinned HTTP client for a trusted hub.
func NewClient(ctx context.Context, baseURL string, token string, opts clientpkg.Opts) (*Client, error) {
	canonicalBaseURL, err := httpurl.CanonicalHTTPBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	c, err := clientpkg.NewHTTPClientWithBearer(ctx, canonicalBaseURL, token, opts)
	if err != nil {
		return nil, err
	}
	if opts.FederationSigning != nil {
		if err := federationsigning.ConfigureClient(c, canonicalBaseURL, token, *opts.FederationSigning); err != nil {
			return nil, err
		}
	}
	return &Client{
		baseURL:     canonicalBaseURL,
		client:      c,
		tokenDigest: sha256.Sum256([]byte(token)),
	}, nil
}

// PollProjectEvents fetches hub project events strictly after afterID.
func (c *Client) PollProjectEvents(
	ctx context.Context, hubProjectID, afterID int64, limit int,
) (api.PollEventsBody, error) {
	body, _, err := c.pollProjectEvents(ctx, hubProjectID, "", afterID, limit, 0)
	return body, err
}

// PollIssueEvents reads retained history for an issue currently owned by the
// authenticated hub project, independently of that project's polling cursor.
func (c *Client) PollIssueEvents(ctx context.Context, hubProjectID int64, issueUID string, afterID int64, limit int) (api.PollEventsBody, error) {
	body, _, err := c.pollProjectEvents(ctx, hubProjectID, issueUID, afterID, limit, 0)
	return body, err
}

// pollProjectEvents polls hub project events strictly after afterID. A
// positive wait asks the hub to hold a caught-up poll for up to that long; the
// result then reports the hub's advertised maximum wait, zero when the hub
// does not hold polls.
func (c *Client) pollProjectEvents(
	ctx context.Context, hubProjectID int64, issueUID string, afterID int64, limit int, wait time.Duration,
) (api.PollEventsBody, time.Duration, error) {
	return api.PollEventsBody{}, 0, errors.New("federation polling is not available")
}

// IngestProjectEvents pushes local spoke events into the hub transport
// endpoint.
func (c *Client) IngestProjectEvents(
	ctx context.Context,
	hubProjectID int64,
	events []api.FederationIngestEventEnvelope,
) (api.FederationIngestEventsBody, error) {
	return c.IngestProjectEventsWithOptions(ctx, hubProjectID, events, IngestProjectEventsOptions{})
}

// IngestProjectEventsOptions carries optional metadata for federation ingest.
type IngestProjectEventsOptions struct {
	MoveSourceTokens           map[string]string
	AdoptionBaseline           string
	AdoptionBaselineEndEventID int64
}

// IngestProjectEventsWithOptions pushes local spoke events with optional
// transport metadata used by chunked adoption baselines.
func (c *Client) IngestProjectEventsWithOptions(
	ctx context.Context,
	hubProjectID int64,
	events []api.FederationIngestEventEnvelope,
	opts IngestProjectEventsOptions,
) (api.FederationIngestEventsBody, error) {
	return api.FederationIngestEventsBody{}, errors.New("federation ingest is not available")
}

// ProjectFederation fetches the hub metadata needed to bind a spoke replica.
func (c *Client) ProjectFederation(ctx context.Context, hubProjectID int64) (api.ProjectFederationBody, error) {
	body, _, err := c.projectFederationFeatures(ctx, hubProjectID)
	return body, err
}
func (c *Client) projectFederationFeatures(ctx context.Context, hubProjectID int64) (api.ProjectFederationBody, string, error) {
	return api.ProjectFederationBody{}, "", errors.New("federation metadata is not available")
}

// RelayReset retrieves a retained root-signed checkpoint through the same
// origin-pinned enrollment credential as ordinary relay delivery.
func (c *Client) RelayReset(context.Context, int64) (db.RelayResetCheckpoint, error) {
	return db.RelayResetCheckpoint{}, errors.New("federation relay is not available")
}

// OfferRelayDeliveries polls the existing enrolled connection. Offers retain
// their identity until the receiver commits and acknowledges their prefix.
func (c *Client) OfferRelayDeliveries(context.Context, int64, string, int, ...int64) (db.RelayBatch, error) {
	return db.RelayBatch{}, errors.New("federation relay is not available")
}

// AcceptRelayDeliveries submits an authenticated hop batch and returns its accepted prefix.
func (c *Client) AcceptRelayDeliveries(context.Context, int64, db.RelayBatch) (db.RelayAcceptance, error) {
	return db.RelayAcceptance{}, errors.New("federation relay is not available")
}

// DownloadRelayArtifacts requests complete misses from the same origin-pinned
// relay route. The sender resolves each digest against its emitted epoch.
func (c *Client) DownloadRelayArtifacts(context.Context, int64, int64, []string) (db.RelayBatch, error) {
	return db.RelayBatch{}, errors.New("federation relay is not available")
}

// AckRelayDeliveries acknowledges an exact emitted prefix for the authenticated hop.
func (c *Client) AckRelayDeliveries(context.Context, int64, int64, string, db.RelayAcceptance) error {
	return errors.New("federation relay is not available")
}

// Keep the replication error body bounded before the generated runtime reads it.
type replicationDoer struct{ client *http.Client }

func (d replicationDoer) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req.Header.Set(db.EventFeaturesHeader, db.SupportedEventFeatures)
	resp, err := d.client.Do(req.WithContext(ctx)) //nolint:gosec // G704: generated replication routes use the explicitly configured trusted federation hub.
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		failure := &HubStatusError{
			Path: req.URL.Path, StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body)),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
		if resp.StatusCode == http.StatusConflict && federationHubErrorCode(string(body)) == "relay_cross_project_links" {
			canonical, _ := json.Marshal(api.NewRelayCrossProjectLinksError("hub").Envelope())
			failure.Body = string(canonical)
			failure.relayCrossProjectLinks = true
		}
		return nil, failure
	}
	if err := db.RequireEventFeatures(db.SupportedEventFeatures, resp.Header.Get(db.RequiredEventFeaturesHeader)); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

func decodeReplicationResponse(resp *http.Response, body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode hub %s response: %w", resp.Request.URL.Path, err)
	}
	return nil
}

func noRedirects(client *http.Client) *http.Client {
	pinned := *client
	pinned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &pinned
}
