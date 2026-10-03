package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Omar2709/pulseops/internal/trading"
)

// Verify that GET retrieves the latest order state
// after a subsequent matching operation.
func TestGetOrderHTTPReturnsUpdatedState(t *testing.T) {
	router := NewRouter(trading.NewService())

	submit := func(body string) {
		t.Helper()

		request := httptest.NewRequest(
			http.MethodPost,
			"/v1/orders",
			strings.NewReader(body),
		)

		request.Header.Set(
			"Content-Type",
			"application/json",
		)

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusCreated {
			t.Fatalf(
				"expected 201, got %d: %s",
				recorder.Code,
				recorder.Body.String(),
			)
		}
	}

	get := func(id string) orderResponse {
		t.Helper()

		request := httptest.NewRequest(
			http.MethodGet,
			"/v1/orders/"+id,
			nil,
		)

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf(
				"expected 200, got %d: %s",
				recorder.Code,
				recorder.Body.String(),
			)
		}

		var response orderResponse

		if err := json.Unmarshal(
			recorder.Body.Bytes(),
			&response,
		); err != nil {
			t.Fatalf("decoding response: %v", err)
		}

		return response
	}

	// Create an initial SELL order.
	submit(`{
		"id": "sell-001",
		"symbol": "BTCUSD",
		"side": "SELL",
		"price": "60000.00",
		"quantity": "0.05"
	}`)

	initial := get("sell-001")

	if initial.Status != "OPEN" {
		t.Fatalf(
			"expected OPEN, got %q",
			initial.Status,
		)
	}

	// Create a compatible BUY order to trigger matching.
	submit(`{
		"id": "buy-001",
		"symbol": "BTCUSD",
		"side": "BUY",
		"price": "61000.00",
		"quantity": "0.05"
	}`)

	// Retrieve the original SELL order again.
	updated := get("sell-001")

	if updated.Status != "FILLED" {
		t.Errorf(
			"expected FILLED, got %q",
			updated.Status,
		)
	}

	if updated.FilledUnits != 5_000_000 {
		t.Errorf(
			"expected 5000000 filled units, got %d",
			updated.FilledUnits,
		)
	}

	if updated.RemainingUnits != 0 {
		t.Errorf(
			"expected zero remaining units, got %d",
			updated.RemainingUnits,
		)
	}

	// Verify that the previous HTTP response
	// remains independent of the current state.
	if initial.Status != "OPEN" {
		t.Error("initial response changed unexpectedly")
	}
}

// Verify that an unknown order returns a standardized
// JSON error with HTTP 404.
func TestGetOrderHTTPReturnsNotFound(t *testing.T) {
	router := NewRouter(trading.NewService())

	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/orders/unknown-order",
		nil,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected 404, got %d",
			recorder.Code,
		)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf(
			"expected JSON response, got %q",
			got,
		)
	}

	var response errorResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&response,
	); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if response.Error != "order not found" {
		t.Errorf(
			"unexpected error: %q",
			response.Error,
		)
	}
}

// Verify that the GET route rejects unsupported methods
// and advertises the GET and HEAD methods it accepts.
func TestGetOrderHTTPRejectsUnsupportedMethod(
	t *testing.T,
) {
	router := NewRouter(trading.NewService())

	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/orders/test-001",
		nil,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"expected 405, got %d",
			recorder.Code,
		)
	}

	allow := recorder.Header().Get("Allow")

	// GET patterns also support HEAD in Go's ServeMux.
	for _, method := range []string{"GET", "HEAD"} {
		found := false

		for _, allowed := range strings.Split(allow, ",") {
			if strings.TrimSpace(allowed) == method {
				found = true
				break
			}
		}

		if !found {
			t.Errorf(
				"expected Allow header to contain %q, got %q",
				method,
				allow,
			)
		}
	}
}
