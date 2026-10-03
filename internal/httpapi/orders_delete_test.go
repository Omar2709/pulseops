package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Omar2709/pulseops/internal/trading"
)

func submitCancellationTestOrder(
	t *testing.T,
	router http.Handler,
	body string,
) {
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

func deleteCancellationTestOrder(
	router http.Handler,
	id string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodDelete,
		"/v1/orders/"+id,
		nil,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func TestCancelOrderHTTPReturnsCancelledOrder(
	t *testing.T,
) {
	router := NewRouter(trading.NewService())

	submitCancellationTestOrder(
		t,
		router,
		`{
            "id": "buy-001",
            "symbol": "BTCUSD",
            "side": "BUY",
            "price": "60000.00",
            "quantity": "0.05"
        }`,
	)

	recorder := deleteCancellationTestOrder(
		router,
		"buy-001",
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("unexpected Content-Type: %q", got)
	}

	var cancelled orderResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&cancelled,
	); err != nil {
		t.Fatalf("decoding cancellation response: %v", err)
	}

	if cancelled.Status != "CANCELLED" {
		t.Errorf(
			"expected CANCELLED, got %q",
			cancelled.Status,
		)
	}

	if cancelled.FilledUnits != 0 {
		t.Errorf(
			"expected zero filled units, got %d",
			cancelled.FilledUnits,
		)
	}

	if cancelled.RemainingUnits != 5_000_000 {
		t.Errorf(
			"unexpected remaining quantity: %d",
			cancelled.RemainingUnits,
		)
	}

	// The cancelled order must remain available through GET.
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/orders/buy-001",
		nil,
	)

	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, request)

	if getRecorder.Code != http.StatusOK {
		t.Fatalf(
			"expected GET to return 200, got %d",
			getRecorder.Code,
		)
	}

	var stored orderResponse

	if err := json.Unmarshal(
		getRecorder.Body.Bytes(),
		&stored,
	); err != nil {
		t.Fatalf("decoding stored order: %v", err)
	}

	if stored.Status != "CANCELLED" {
		t.Errorf(
			"expected stored order to be CANCELLED, got %q",
			stored.Status,
		)
	}

	// Cancelling the same order again must return 409.
	second := deleteCancellationTestOrder(
		router,
		"buy-001",
	)

	if second.Code != http.StatusConflict {
		t.Errorf(
			"expected 409, got %d",
			second.Code,
		)
	}
}

func TestCancelOrderHTTPReturnsNotFound(
	t *testing.T,
) {
	router := NewRouter(trading.NewService())

	recorder := deleteCancellationTestOrder(
		router,
		"unknown-order",
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected 404, got %d",
			recorder.Code,
		)
	}

	var response errorResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&response,
	); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}

	if response.Error != "order not found" {
		t.Errorf(
			"unexpected error: %q",
			response.Error,
		)
	}
}

func TestCancelOrderHTTPRejectsFilledOrder(
	t *testing.T,
) {
	router := NewRouter(trading.NewService())

	submitCancellationTestOrder(
		t,
		router,
		`{
            "id": "sell-001",
            "symbol": "BTCUSD",
            "side": "SELL",
            "price": "60000.00",
            "quantity": "0.05"
        }`,
	)

	submitCancellationTestOrder(
		t,
		router,
		`{
            "id": "buy-001",
            "symbol": "BTCUSD",
            "side": "BUY",
            "price": "61000.00",
            "quantity": "0.05"
        }`,
	)

	recorder := deleteCancellationTestOrder(
		router,
		"sell-001",
	)

	if recorder.Code != http.StatusConflict {
		t.Fatalf(
			"expected 409, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var response errorResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&response,
	); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}

	if response.Error != "order cannot be cancelled" {
		t.Errorf(
			"unexpected error: %q",
			response.Error,
		)
	}
}

// A partially executed order retains its historical fill after cancellation,
// and its unfilled quantity must no longer participate in matching.
func TestCancelOrderHTTPPreservesPartialFill(
	t *testing.T,
) {
	router := NewRouter(trading.NewService())

	post := func(body string) orderResponse {
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

		var response orderResponse

		if err := json.Unmarshal(
			recorder.Body.Bytes(),
			&response,
		); err != nil {
			t.Fatalf("decoding POST response: %v", err)
		}

		return response
	}

	// Register a SELL order for 0.05 BTC.
	post(`{
		"id": "sell-001",
		"symbol": "BTCUSD",
		"side": "SELL",
		"price": "60000.00",
		"quantity": "0.05"
	}`)

	// Execute 0.02 BTC, leaving 0.03 BTC pending.
	post(`{
		"id": "buy-001",
		"symbol": "BTCUSD",
		"side": "BUY",
		"price": "61000.00",
		"quantity": "0.02"
	}`)

	// Cancel the remaining SELL quantity.
	recorder := deleteCancellationTestOrder(
		router,
		"sell-001",
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var cancelled orderResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&cancelled,
	); err != nil {
		t.Fatalf("decoding cancellation: %v", err)
	}

	if cancelled.Status != "CANCELLED" {
		t.Errorf(
			"expected CANCELLED, got %q",
			cancelled.Status,
		)
	}

	if cancelled.FilledUnits != 2_000_000 {
		t.Errorf(
			"expected 2000000 filled units, got %d",
			cancelled.FilledUnits,
		)
	}

	if cancelled.RemainingUnits != 3_000_000 {
		t.Errorf(
			"expected 3000000 remaining units, got %d",
			cancelled.RemainingUnits,
		)
	}

	// GET must return the same current state.
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/orders/sell-001",
		nil,
	)

	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, request)

	if getRecorder.Code != http.StatusOK {
		t.Fatalf(
			"expected GET 200, got %d",
			getRecorder.Code,
		)
	}

	var stored orderResponse

	if err := json.Unmarshal(
		getRecorder.Body.Bytes(),
		&stored,
	); err != nil {
		t.Fatalf("decoding GET response: %v", err)
	}

	if stored.Status != "CANCELLED" ||
		stored.FilledUnits != 2_000_000 ||
		stored.RemainingUnits != 3_000_000 {
		t.Errorf(
			"unexpected stored order after cancellation: %+v",
			stored,
		)
	}

	// A new BUY must not match the cancelled SELL.
	next := post(`{
		"id": "buy-002",
		"symbol": "BTCUSD",
		"side": "BUY",
		"price": "61000.00",
		"quantity": "0.03"
	}`)

	if next.Status != "OPEN" ||
		next.FilledUnits != 0 ||
		next.RemainingUnits != 3_000_000 {
		t.Errorf(
			"new BUY unexpectedly matched: %+v",
			next,
		)
	}
}

// Only GET, HEAD and DELETE are allowed on a specific order resource.
func TestCancelOrderHTTPRejectsUnsupportedMethod(
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

	for _, method := range []string{
		"GET",
		"HEAD",
		"DELETE",
	} {
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
