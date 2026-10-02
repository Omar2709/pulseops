package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Omar2709/pulseops/internal/trading"
)

// TestSubmitOrderHTTP verifies successful order submission
// and fixed-point serialization.
func TestSubmitOrderHTTP(t *testing.T) {
	router := NewRouter(trading.NewService())

	body := `{
		"id": "buy-001",
		"symbol": "BTCUSD",
		"side": "BUY",
		"price": "60000.25",
		"quantity": "0.05"
	}`

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

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf(
			"unexpected Content-Type: %q",
			got,
		)
	}

	var response orderResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&response,
	); err != nil {
		t.Fatalf(
			"decoding response: %v",
			err,
		)
	}

	if response.ID != "buy-001" {
		t.Errorf(
			"unexpected order ID: %q",
			response.ID,
		)
	}

	if response.Symbol != "BTCUSD" {
		t.Errorf(
			"unexpected symbol: %q",
			response.Symbol,
		)
	}

	if response.Side != "BUY" {
		t.Errorf(
			"unexpected side: %q",
			response.Side,
		)
	}

	if response.Status != "OPEN" {
		t.Errorf(
			"unexpected status: %q",
			response.Status,
		)
	}

	if response.PriceUnits != 6_000_025 {
		t.Errorf(
			"unexpected price units: %d",
			response.PriceUnits,
		)
	}

	if response.QuantityUnits != 5_000_000 {
		t.Errorf(
			"unexpected quantity units: %d",
			response.QuantityUnits,
		)
	}

	if response.FilledUnits != 0 {
		t.Errorf(
			"expected zero filled units, got %d",
			response.FilledUnits,
		)
	}

	if response.RemainingUnits != 5_000_000 {
		t.Errorf(
			"unexpected remaining units: %d",
			response.RemainingUnits,
		)
	}

	if response.CreatedAt == "" {
		t.Error("expected created_at timestamp")
	}

	if response.UpdatedAt == "" {
		t.Error("expected updated_at timestamp")
	}

	// Verify that int64 values are serialized
	// as JSON strings to preserve precision.
	var raw map[string]any

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&raw,
	); err != nil {
		t.Fatalf(
			"decoding raw response: %v",
			err,
		)
	}

	if got := raw["price_units"]; got != "6000025" {
		t.Errorf(
			"expected price_units as string, got %v",
			got,
		)
	}

	if got := raw["quantity_units"]; got != "5000000" {
		t.Errorf(
			"expected quantity_units as string, got %v",
			got,
		)
	}

	if got := raw["filled_units"]; got != "0" {
		t.Errorf(
			"expected filled_units as string, got %v",
			got,
		)
	}

	if got := raw["remaining_units"]; got != "5000000" {
		t.Errorf(
			"expected remaining_units as string, got %v",
			got,
		)
	}
}

// TestSubmitOrderHTTPRejectsInvalidRequests verifies
// that invalid requests return the expected HTTP status.
//
// Each test uses an independent trading service
// to prevent state contamination between cases.
func TestSubmitOrderHTTPRejectsInvalidRequests(
	t *testing.T,
) {
	tests := []struct {
		name       string
		body       string
		statusCode int
	}{
		{
			name:       "malformed JSON",
			body:       `{"id":`,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "unknown JSON field",
			body: `{
				"id": "buy-001",
				"symbol": "BTCUSD",
				"side": "BUY",
				"price": "60000.25",
				"quantity": "0.05",
				"unexpected": true
			}`,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "invalid side",
			body: `{
				"id": "buy-001",
				"symbol": "BTCUSD",
				"side": "INVALID",
				"price": "60000.25",
				"quantity": "0.05"
			}`,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "zero price",
			body: `{
				"id": "buy-001",
				"symbol": "BTCUSD",
				"side": "BUY",
				"price": "0",
				"quantity": "0.05"
			}`,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "zero quantity",
			body: `{
				"id": "buy-001",
				"symbol": "BTCUSD",
				"side": "BUY",
				"price": "60000.25",
				"quantity": "0"
			}`,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "missing ID",
			body: `{
				"symbol": "BTCUSD",
				"side": "BUY",
				"price": "60000.25",
				"quantity": "0.05"
			}`,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "missing symbol",
			body: `{
				"id": "buy-001",
				"side": "BUY",
				"price": "60000.25",
				"quantity": "0.05"
			}`,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "numeric price instead of string",
			body: `{
				"id": "buy-001",
				"symbol": "BTCUSD",
				"side": "BUY",
				"price": 60000.25,
				"quantity": "0.05"
			}`,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "multiple JSON objects",
			body: `{
				"id": "buy-001",
				"symbol": "BTCUSD",
				"side": "BUY",
				"price": "60000.25",
				"quantity": "0.05"
			} {}`,
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "request body too large",
			body:       strings.Repeat(" ", maxOrderBodyBytes+1),
			statusCode: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(trading.NewService())

			request := httptest.NewRequest(
				http.MethodPost,
				"/v1/orders",
				strings.NewReader(tt.body),
			)
			request.Header.Set(
				"Content-Type",
				"application/json",
			)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.statusCode {
				t.Fatalf(
					"expected %d, got %d: %s",
					tt.statusCode,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf(
					"expected JSON response, got %q",
					got,
				)
			}
		})
	}
}

// TestSubmitOrderHTTPRejectsDuplicateID verifies
// that submitting the same order ID twice returns
// HTTP 409 without replacing the existing order.
func TestSubmitOrderHTTPRejectsDuplicateID(
	t *testing.T,
) {
	router := NewRouter(trading.NewService())

	body := `{
		"id": "buy-001",
		"symbol": "BTCUSD",
		"side": "BUY",
		"price": "60000.25",
		"quantity": "0.05"
	}`

	submit := func() *httptest.ResponseRecorder {
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

		return recorder
	}

	first := submit()

	if first.Code != http.StatusCreated {
		t.Fatalf(
			"expected first request to return 201, got %d: %s",
			first.Code,
			first.Body.String(),
		)
	}

	second := submit()

	if second.Code != http.StatusConflict {
		t.Fatalf(
			"expected duplicate request to return 409, got %d: %s",
			second.Code,
			second.Body.String(),
		)
	}

	if got := second.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf(
			"expected JSON error response, got %q",
			got,
		)
	}

	var response errorResponse

	if err := json.NewDecoder(
		second.Body,
	).Decode(&response); err != nil {
		t.Fatalf(
			"decoding error response: %v",
			err,
		)
	}

	if response.Error != "order already exists" {
		t.Errorf(
			"unexpected error message: %q",
			response.Error,
		)
	}
}

// TestSubmitOrderHTTPRejectsUnsupportedMediaType
// verifies that the endpoint requires application/json.
func TestSubmitOrderHTTPRejectsUnsupportedMediaType(
	t *testing.T,
) {
	tests := []struct {
		name        string
		contentType string
	}{
		{
			name:        "plain text",
			contentType: "text/plain",
		},
		{
			name:        "missing content type",
			contentType: "",
		},
		{
			name:        "invalid media type",
			contentType: "invalid/type; charset=\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(trading.NewService())

			request := httptest.NewRequest(
				http.MethodPost,
				"/v1/orders",
				strings.NewReader(`{}`),
			)

			if tt.contentType != "" {
				request.Header.Set(
					"Content-Type",
					tt.contentType,
				)
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusUnsupportedMediaType {
				t.Fatalf(
					"expected 415, got %d: %s",
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf(
					"expected JSON response, got %q",
					got,
				)
			}

			var response errorResponse

			if err := json.NewDecoder(
				recorder.Body,
			).Decode(&response); err != nil {
				t.Fatalf(
					"decoding error response: %v",
					err,
				)
			}

			if response.Error == "" {
				t.Error("expected non-empty error message")
			}
		})
	}
}

// TestSubmitOrderHTTPRejectsGet verifies that
// the router only permits POST for /v1/orders.
func TestSubmitOrderHTTPRejectsGet(t *testing.T) {
	router := NewRouter(trading.NewService())

	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/orders",
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

	if got := recorder.Header().Get("Allow"); got != "POST" {
		t.Errorf(
			"expected Allow header %q, got %q",
			"POST",
			got,
		)
	}
}

// TestSubmitOrderHTTPMatchesCrossingOrders verifies
// matching between two HTTP requests sharing
// the same trading service.
func TestSubmitOrderHTTPMatchesCrossingOrders(
	t *testing.T,
) {
	router := NewRouter(trading.NewService())

	submit := func(body string) (
		int,
		orderResponse,
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

		var response orderResponse

		if recorder.Code == http.StatusCreated {
			if err := json.NewDecoder(
				recorder.Body,
			).Decode(&response); err != nil {
				t.Fatalf(
					"decoding order: %v",
					err,
				)
			}
		}

		return recorder.Code, response
	}

	// First request: create a resting SELL order.
	sellBody := `{
		"id": "sell-001",
		"symbol": "BTCUSD",
		"side": "SELL",
		"price": "60000.00",
		"quantity": "0.05"
	}`

	status, sell := submit(sellBody)

	if status != http.StatusCreated {
		t.Fatalf(
			"expected SELL creation to return 201, got %d",
			status,
		)
	}

	if sell.ID != "sell-001" {
		t.Errorf(
			"unexpected SELL ID: %q",
			sell.ID,
		)
	}

	if sell.Status != "OPEN" {
		t.Errorf(
			"expected SELL to be OPEN, got %q",
			sell.Status,
		)
	}

	if sell.FilledUnits != 0 {
		t.Errorf(
			"expected SELL filled units 0, got %d",
			sell.FilledUnits,
		)
	}

	if sell.RemainingUnits != 5_000_000 {
		t.Errorf(
			"expected SELL remaining units 5000000, got %d",
			sell.RemainingUnits,
		)
	}

	// Second request: create a crossing BUY order.
	buyBody := `{
		"id": "buy-001",
		"symbol": "BTCUSD",
		"side": "BUY",
		"price": "61000.00",
		"quantity": "0.05"
	}`

	status, buy := submit(buyBody)

	if status != http.StatusCreated {
		t.Fatalf(
			"expected BUY creation to return 201, got %d",
			status,
		)
	}

	if buy.ID != "buy-001" {
		t.Errorf(
			"unexpected BUY ID: %q",
			buy.ID,
		)
	}

	if buy.Status != "FILLED" {
		t.Errorf(
			"expected BUY to be FILLED, got %q",
			buy.Status,
		)
	}

	if buy.FilledUnits != 5_000_000 {
		t.Errorf(
			"expected filled quantity 5000000, got %d",
			buy.FilledUnits,
		)
	}

	if buy.RemainingUnits != 0 {
		t.Errorf(
			"expected no remaining quantity, got %d",
			buy.RemainingUnits,
		)
	}
}
