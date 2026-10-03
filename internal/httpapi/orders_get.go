package httpapi

import (
	"errors"
	"log"
	"net/http"

	"github.com/Omar2709/pulseops/internal/trading"
)

// GetOrder retrieves the current state of a registered order.
func (h *Handler) GetOrder(
	w http.ResponseWriter,
	r *http.Request,
) {
	id := r.PathValue("id")

	order, err := h.trading.GetOrder(id)
	if err != nil {
		if errors.Is(err, trading.ErrOrderNotFound) {
			writeError(
				w,
				http.StatusNotFound,
				"order not found",
			)
			return
		}

		log.Printf("get order failed: %v", err)

		writeError(
			w,
			http.StatusInternalServerError,
			"internal server error",
		)
		return
	}

	response := orderResponseFromSnapshot(order)

	if err := writeJSON(
		w,
		http.StatusOK,
		response,
	); err != nil {
		log.Printf("write order response: %v", err)
	}
}
