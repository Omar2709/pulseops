package httpapi

import (
	"encoding/json"
	"net/http"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	data any,
) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_, err = w.Write(payload)

	return err
}

func writeError(
	w http.ResponseWriter,
	status int,
	message string,
) {
	if err := writeJSON(
		w,
		status,
		errorResponse{
			Error: message,
		},
	); err != nil {
		return
	}
}
