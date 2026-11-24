package main

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
)

func externalCLITransportError[T any](resp *T, callErr error) error {
	if resp != nil {
		return nil
	}
	if callErr != nil {
		return callErr
	}
	return &cliError{
		Message: "daemon request returned no response", Kind: kindInternal, ExitCode: ExitInternal,
	}
}

func externalCLIResponseError(status int, body []byte, callErr error) error {
	if status >= http.StatusBadRequest {
		var envelope struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Code != "" && envelope.Error.Message != "" {
			return apiErrFromBody(status, body)
		}
		message := fmt.Sprintf("daemon request failed (HTTP %d)", status)
		if statusText := http.StatusText(status); statusText != "" {
			message = fmt.Sprintf("daemon request failed (HTTP %d %s)", status, statusText)
		}
		return &cliError{
			HTTPStatus: status,
			Message:    message, Kind: kindForStatus(status), ExitCode: mapStatusToExit(status, ""),
		}
	}
	return callErr
}

func externalCLIValidation(message string) error {
	return &cliError{Message: message, Kind: kindValidation, ExitCode: ExitValidation}
}
