package poomsae_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae"
	"github.com/stretchr/testify/require"
)

func TestServiceDoctorRequiresManagePolicyAndHonorsHostDenial(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "permitted", true: "denied"}[denied], func(t *testing.T) {
			controller := &recordingAccessController{}
			if denied {
				controller.err = poomsae.ErrAccessDenied
			}
			service, err := poomsae.New(t.Context(), poomsae.Config{DSN: filepath.Join(t.TempDir(), "service.db"), Access: controller})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, service.Close()) })
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/doctor", nil)
			req = req.WithContext(poomsae.WithPrincipal(req.Context(), poomsae.Principal{Subject: "operator", Actor: "operator"}))
			rr := httptest.NewRecorder()
			service.Handler().ServeHTTP(rr, req)
			if denied {
				require.Equal(t, 404, rr.Code)
			} else {
				require.Equal(t, 200, rr.Code, rr.Body.String())
			}
			require.Len(t, controller.requests, 1)
			policy := controller.requests[0].Operation.Policy
			require.EqualValues(t, "integration_administration", policy.Kind)
			require.EqualValues(t, "manage", policy.Capability)
			require.False(t, policy.Mutation)
		})
	}
}
