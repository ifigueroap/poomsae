package poomsae_test

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ifigueroap/poomsae"
)

func purgeTestIssueStatus(t *testing.T, service *poomsae.Service, method string, projectID int64, suffix, body string) int {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method,
		"/api/v1/projects/"+strconv.FormatInt(projectID, 10)+"/issues"+suffix, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	return response.Code
}

// checkServicePurgeArchivedHubProject covers the host workflow after a hub
// project is replaced: archive it, then purge it so its identity and tasks no
// longer occupy the hub.
