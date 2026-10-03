package httpapi

import (
	"errors"
	"log"
	"net/http"

	"github.com/Omar2709/pulseops/internal/trading"
)

// CancelOrder cancels an open or partially filled order.
func (h *Handler) CancelOrder(
	w http.ResponseWriter,
	r *http.Request,
) {
	id := r.PathValue("id")

	order, err := h.trading.CancelOrder(id)
	if err != nil {
		switch {
		case errors.Is(err, trading.ErrOrderNotFound):
			writeError(
				w,
				http.StatusNotFound,
				"order not found",
			)

		case errors.Is(err, trading.ErrOrderNotCancellable):
			writeError(
				w,
				http.StatusConflict,
				"order cannot be cancelled",
			)

		default:
			log.Printf("cancel order failed: %v", err)

			writeError(
				w,
				http.StatusInternalServerError,
				"internal server error",
			)
		}

		return
	}

	response := orderResponseFromSnapshot(order)

	if err := writeJSON(
		w,
		http.StatusOK,
		response,
	); err != nil {
		log.Printf("write cancellation response: %v", err)
	}
}
