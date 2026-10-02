package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Omar2709/pulseops/internal/trading"
)

func TestRouterReturnsNotFoundForUnknownRoute(
	t *testing.T,
) {
	router := NewRouter(trading.NewService())

	request := httptest.NewRequest(
		http.MethodGet,
		"/unknown",
		nil,
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf(
			"expected status %d, got %d",
			http.StatusNotFound,
			recorder.Code,
		)
	}
}
