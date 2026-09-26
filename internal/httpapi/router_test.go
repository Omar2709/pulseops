package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterReturnsNotFoundForUnknownRoute(t *testing.T) {
	router := NewRouter()

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
