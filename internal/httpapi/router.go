package httpapi

import (
	"net/http"

	"github.com/Omar2709/pulseops/internal/trading"
)

func NewRouter(
	service *trading.Service,
) http.Handler {
	mux := http.NewServeMux()

	handler := NewHandler(service)

	mux.HandleFunc(
		"GET /healthz",
		healthHandler,
	)

	mux.HandleFunc(
		"POST /v1/orders",
		handler.SubmitOrder,
	)

	return mux
}
