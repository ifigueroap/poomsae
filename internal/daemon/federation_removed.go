package daemon

import (
	"context"
	"errors"
	"net/http"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/db"
)

var (
	ErrFederationReplicaCredentialConflict   = errors.New("federation replica credential conflict")
	ErrFederationProviderIntentChangeBlocked = errors.New("federation provider intent change blocked")
	ErrFederationReplicaBindingConflict      = errors.New("federation replica binding conflict")
	ErrFederationReplicaCredentialIO         = errors.New("federation replica credential io")
	ErrFederationProviderStorage             = errors.New("federation provider storage")
	ErrFederationReplicaLeavePending         = errors.New("federation replica leave pending")
	ErrFederationReplicaReservationChanged   = errors.New("federation replica reservation changed")
)

type FederationReplicaManagedReservation struct {
	ProjectUID string
	Expected   config.FederationCredential
}

type FederationReplicaCredentialRekeySource struct {
	ProjectUID string
	Expected   config.FederationCredential
}

type EnsureFederationReplicaParams struct {
	HubURL               string
	HubProjectID         int64
	HubProjectUID        string
	ProjectName          string
	ReplayHorizonEventID int64
	Credential           config.FederationCredential
	PushEnabled          bool
	AdoptExisting        bool
	AttachEmpty          bool
	ManagedReservation   *FederationReplicaManagedReservation
	ProjectEventSink     func(db.Event)
	ProjectResetSink     func(projectID, cursor int64)
	ReplacesBoundActor   string
	CredentialRekey      *FederationReplicaCredentialRekeySource
}

func FederationReplicaMappingSuppressed(db.Storage, string) bool { return true }

func AuthorizeFederationProvider(context.Context, db.Storage, config.FederationCredentialStore, config.CatalogDaemonConfig, config.FederationProjectConfig) (config.FederationManagedCredentialReservation, error) {
	return config.FederationManagedCredentialReservation{}, ErrFederationReplicaCredentialConflict
}

func BeginFederationReplicaHubOperation(context.Context, db.Storage, config.FederationCredentialStore, string, config.FederationManagedCredentialReservation) (func(context.Context, int64) (bool, error), error) {
	return nil, ErrFederationReplicaCredentialConflict
}

func RecordFederationReplicaPendingEnrollment(context.Context, config.FederationCredentialStore, string, int64) (bool, error) {
	return false, ErrFederationReplicaCredentialConflict
}

func BeginFederationReplicaLeaveRecovery(context.Context, db.Storage, config.FederationManagedCredentialStore, string, config.FederationManagedCredentialReservation) (func(context.Context, int64) (bool, error), error) {
	return nil, ErrFederationReplicaLeavePending
}

type ReserveFederationReplicaCredentialParams struct {
	HubProjectUID string
	ProjectName   string
	Credential    config.FederationCredential
	ExpectedBound bool
}

func ReserveFederationReplicaCredential(context.Context, db.Storage, config.FederationManagedCredentialStore, ReserveFederationReplicaCredentialParams) error {
	return ErrFederationReplicaCredentialConflict
}

func EnsureFederationReplica(context.Context, db.Storage, config.FederationCredentialStore, func(), EnsureFederationReplicaParams) (db.FederationBinding, error) {
	return db.FederationBinding{}, ErrFederationReplicaCredentialConflict
}

func ReleaseRemovedFederationProvider(context.Context, db.Storage, config.FederationManagedCredentialStore, config.FederationManagedCredentialReservation) (config.FederationManagedCredentialReservation, db.Project, error) {
	return config.FederationManagedCredentialReservation{}, db.Project{}, ErrFederationReplicaCredentialConflict
}

func LeaveFederationReplica(context.Context, db.Storage, config.FederationManagedCredentialStore, func(), int64) (db.FederationBinding, error) {
	return db.FederationBinding{}, ErrFederationReplicaCredentialConflict
}

// Federation enrollment is not part of this daemon. The types below remain so
// claim, attachment, and cron call sites compile, and every authorization
// attempt is refused.

var errHostFederationAccessUnavailable = errors.New("host federation access unavailable")

var ErrHostFederationAdmissionLimited = errors.New("host federation request admission limited")

type HostFederationAccessRequest struct {
	Enrollment           db.FederationEnrollment
	EnrollmentProjectUID string
	Project              db.Project
	Capability           string
	Operation            HostFederationOperation
}

type HostFederationAccessDecision struct {
	TransactionFence db.TransactionFence
}

type HostFederationAccessController interface {
	AuthorizeFederation(context.Context, HostFederationAccessRequest) (HostFederationAccessDecision, error)
}

type HostFederationOperation struct {
	ID       string
	Mutation bool
}

func federationTransportOperation(operationID string) HostFederationOperation {
	return HostFederationOperation{ID: operationID, Mutation: true}
}

type federationPrincipal struct {
	RelayBindingUID      string
	RelayProtocolVersion int
	ScopedProjectID      int64
	SpokeInstanceUID     string
	Actor                string
}

type federationAuthorization struct {
	principal        federationPrincipal
	transactionFence db.TransactionFence
}

func authorizeFederationRequest(
	ctx context.Context,
	_ ServerConfig,
	_ string,
	_ int64,
	_ string,
	_ HostFederationOperation,
) (context.Context, federationPrincipal, error) {
	return ctx, federationPrincipal{}, federationCredentialDenied()
}

func authorizeFederationRequestForCapabilities(
	ctx context.Context,
	_ ServerConfig,
	_ string,
	_ int64,
	_ []string,
	_ HostFederationOperation,
) (context.Context, federationPrincipal, error) {
	return ctx, federationPrincipal{}, federationCredentialDenied()
}

func evaluateFederationRequest(
	_ context.Context,
	_ ServerConfig,
	_ string,
	_ int64,
	_ string,
	_ HostFederationOperation,
) (federationAuthorization, error) {
	return federationAuthorization{}, federationCredentialDenied()
}

func federationBridgeHubRejection(status int) *api.APIError {
	data := map[string]any{"hub_status": status}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusPreconditionRequired:
		return api.NewError(http.StatusBadGateway, "hub_credential_rejected", "selected hub rejected the presented credential", "check the hub account, its project access and the hub's signing policy", data)
	case status < 500:
		status = http.StatusBadGateway
	}
	return api.NewError(status, "hub_request_rejected", "selected hub rejected the request", "check account access and selected project", data)
}

func federationCredentialDenied() error {
	return api.NewError(http.StatusForbidden, "auth_invalid",
		"federation credential is not currently authorized", "", nil)
}

func (ServerConfig) federationCredentialStore() config.FederationCredentialStore {
	return config.DefaultFederationCredentialStore()
}
