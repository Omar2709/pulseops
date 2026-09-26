package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	recorder := httptest.NewRecorder()

	data := healthResponse{
		Status: "ok",
	}

	err := writeJSON(
		recorder,
		http.StatusOK,
		data,
	)

	if err != nil {
		t.Fatalf("writing JSON response: %v", err)
	}

	if recorder.Code != http.StatusOK {
		t.Errorf(
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
		t.Fatalf("decoding response: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf(
			"expected status %q, got %q",
			"ok",
			response.Status,
		)
	}
}

func TestWriteError(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeError(
		recorder,
		http.StatusBadRequest,
		"invalid request",
	)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf(
			"expected status %d, got %d",
			http.StatusBadRequest,
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

	var response errorResponse

	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if response.Error != "invalid request" {
		t.Errorf(
			"expected error %q, got %q",
			"invalid request",
			response.Error,
		)
	}
}

func TestWriteJSONReturnsErrorForUnsupportedValue(t *testing.T) {
	recorder := httptest.NewRecorder()

	err := writeJSON(
		recorder,
		http.StatusOK,
		make(chan int),
	)

	if err == nil {
		t.Fatal("expected JSON serialization error")
	}

	if recorder.Body.Len() != 0 {
		t.Errorf(
			"expected empty response body, got %q",
			recorder.Body.String(),
		)
	}
}
