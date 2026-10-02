package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
)

const maxOrderBodyBytes = 16 << 10

type Handler struct {
	trading *trading.Service
}

func NewHandler(service *trading.Service) *Handler {
	return &Handler{trading: service}
}

type submitOrderRequest struct {
	ID       string `json:"id"`
	Symbol   string `json:"symbol"`
	Side     string `json:"side"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

type orderResponse struct {
	ID             string `json:"id"`
	Symbol         string `json:"symbol"`
	Side           string `json:"side"`
	PriceUnits     int64  `json:"price_units,string"`
	QuantityUnits  int64  `json:"quantity_units,string"`
	FilledUnits    int64  `json:"filled_units,string"`
	RemainingUnits int64  `json:"remaining_units,string"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func (h *Handler) SubmitOrder(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Require an application/json request.
	mediaType, _, err := mime.ParseMediaType(
		r.Header.Get("Content-Type"),
	)
	if err != nil || mediaType != "application/json" {
		writeError(
			w,
			http.StatusUnsupportedMediaType,
			"Content-Type must be application/json",
		)
		return
	}

	// Reject request bodies exceeding 16 KiB.
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		maxOrderBodyBytes,
	)
	defer r.Body.Close()

	var request submitOrderRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		writeDecodeError(w, err)
		return
	}

	// Reject additional JSON values or trailing invalid content.
	var extra any

	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			writeError(
				w,
				http.StatusBadRequest,
				"invalid JSON request",
			)
		} else {
			writeDecodeError(w, err)
		}

		return
	}

	// Validate and normalize the order side.
	side, err := orders.ParseSide(request.Side)
	if err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"invalid order side",
		)
		return
	}

	// Convert and validate the price.
	priceUnits, err := parseFixedPoint(
		request.Price,
		orders.PriceScale,
	)
	if err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"invalid price",
		)
		return
	}

	price, err := orders.NewPrice(priceUnits)
	if err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"invalid price",
		)
		return
	}

	// Convert and validate the quantity.
	quantityUnits, err := parseFixedPoint(
		request.Quantity,
		orders.QuantityScale,
	)
	if err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"invalid quantity",
		)
		return
	}

	quantity, err := orders.NewQuantity(quantityUnits)
	if err != nil || quantity == 0 {
		writeError(
			w,
			http.StatusBadRequest,
			"invalid quantity",
		)
		return
	}

	// Submit the validated order.
	result, err := h.trading.SubmitOrder(
		trading.SubmitOrderInput{
			ID:       request.ID,
			Symbol:   request.Symbol,
			Side:     side,
			Price:    price,
			Quantity: quantity,
		},
	)
	if err != nil {
		writeSubmitOrderError(w, err)
		return
	}

	// Build the HTTP response using a domain snapshot.
	order := result.Order

	response := orderResponse{
		ID:             order.ID,
		Symbol:         order.Symbol,
		Side:           string(order.Side),
		PriceUnits:     order.Price.Units(),
		QuantityUnits:  order.Quantity.Units(),
		FilledUnits:    order.FilledQuantity.Units(),
		RemainingUnits: order.RemainingQuantity.Units(),
		Status:         string(order.Status),
		CreatedAt:      order.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:      order.UpdatedAt.Format(time.RFC3339Nano),
	}

	if err := writeJSON(
		w,
		http.StatusCreated,
		response,
	); err != nil {
		log.Printf("write order response: %v", err)
	}
}

func writeDecodeError(
	w http.ResponseWriter,
	err error,
) {
	var tooLarge *http.MaxBytesError

	if errors.As(err, &tooLarge) {
		writeError(
			w,
			http.StatusRequestEntityTooLarge,
			"request body too large",
		)
		return
	}

	writeError(
		w,
		http.StatusBadRequest,
		"invalid JSON request",
	)
}

func writeSubmitOrderError(
	w http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(err, trading.ErrDuplicateOrder):
		writeError(
			w,
			http.StatusConflict,
			"order already exists",
		)

	case errors.Is(err, orders.ErrOrderIDRequired),
		errors.Is(err, orders.ErrOrderSymbolRequired),
		errors.Is(err, orders.ErrInvalidSide),
		errors.Is(err, orders.ErrInvalidPrice),
		errors.Is(err, orders.ErrInvalidQuantity),
		errors.Is(err, orders.ErrOrderQuantityInvalid):
		writeError(
			w,
			http.StatusBadRequest,
			err.Error(),
		)

	default:
		log.Printf("submit order failed: %v", err)

		writeError(
			w,
			http.StatusInternalServerError,
			"internal server error",
		)
	}
}
