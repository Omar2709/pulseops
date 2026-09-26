package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	router := NewRouter()

	request := httptest.NewRequest(
		http.MethodGet,
		"/healthz",
		nil,
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf(
			"expected Content-Type %q, got %q",
			"application/json",
			got,
		)
	}

	var response healthResponse

	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf(
			"expected status %q, got %q",
			"ok",
			response.Status,
		)
	}
}

func TestHealthEndpointRejectsPost(t *testing.T) {
	router := NewRouter()

	request := httptest.NewRequest(
		http.MethodPost,
		"/healthz",
		nil,
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf(
			"expected status %d, got %d",
			http.StatusMethodNotAllowed,
			recorder.Code,
		)
	}
}
