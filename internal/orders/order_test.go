package orders

import (
	"errors"
	"testing"
	"time"
)

func TestNewOrder(t *testing.T) {
	bogotaLocation := time.FixedZone("UTC-5", -5*60*60)

	createdAt := time.Date(
		2026,
		time.September,
		16,
		12,
		0,
		0,
		0,
		bogotaLocation,
	)

	expectedUTC := createdAt.UTC()

	price, err := NewPrice(6_000_025)
	if err != nil {
		t.Fatalf("creating price: %v", err)
	}

	quantity, err := NewQuantity(5_000_000)
	if err != nil {
		t.Fatalf("creating quantity: %v", err)
	}

	order, err := NewOrder(
		"ord_001",
		"btc-usd",
		SideBuy,
		price,
		quantity,
		createdAt,
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if order.ID() != "ord_001" {
		t.Errorf("expected ID %q, got %q", "ord_001", order.ID())
	}

	if order.Symbol() != "BTC-USD" {
		t.Errorf("expected symbol %q, got %q", "BTC-USD", order.Symbol())
	}

	if order.Side() != SideBuy {
		t.Errorf("expected side %q, got %q", SideBuy, order.Side())
	}

	if order.Price() != price {
		t.Errorf("expected price %d, got %d", price, order.Price())
	}

	if order.Quantity() != quantity {
		t.Errorf("expected quantity %d, got %d", quantity, order.Quantity())
	}

	if order.FilledQuantity() != 0 {
		t.Errorf(
			"expected filled quantity %d, got %d",
			0,
			order.FilledQuantity(),
		)
	}

	if order.Status() != OrderStatusPending {
		t.Errorf(
			"expected status %q, got %q",
			OrderStatusPending,
			order.Status(),
		)
	}

	if !order.CreatedAt().Equal(expectedUTC) {
		t.Errorf(
			"expected created at %v, got %v",
			expectedUTC,
			order.CreatedAt(),
		)
	}

	if order.CreatedAt().Location() != time.UTC {
		t.Errorf(
			"expected created at location UTC, got %v",
			order.CreatedAt().Location(),
		)
	}

	if !order.UpdatedAt().Equal(expectedUTC) {
		t.Errorf(
			"expected updated at %v, got %v",
			expectedUTC,
			order.UpdatedAt(),
		)
	}

	if order.UpdatedAt().Location() != time.UTC {
		t.Errorf(
			"expected updated at location UTC, got %v",
			order.UpdatedAt().Location(),
		)
	}
}

func TestNewOrderValidation(t *testing.T) {
	createdAt := time.Date(
		2026,
		time.September,
		16,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	validPrice := Price(6_000_025)
	validQuantity := Quantity(5_000_000)

	tests := []struct {
		name      string
		id        string
		symbol    string
		side      Side
		price     Price
		quantity  Quantity
		createdAt time.Time
		wantErr   error
	}{
		{
			name:      "empty ID",
			id:        "",
			symbol:    "BTC-USD",
			side:      SideBuy,
			price:     validPrice,
			quantity:  validQuantity,
			createdAt: createdAt,
			wantErr:   ErrOrderIDRequired,
		},
		{
			name:      "ID with only spaces",
			id:        "   ",
			symbol:    "BTC-USD",
			side:      SideBuy,
			price:     validPrice,
			quantity:  validQuantity,
			createdAt: createdAt,
			wantErr:   ErrOrderIDRequired,
		},
		{
			name:      "empty symbol",
			id:        "ord_001",
			symbol:    "",
			side:      SideBuy,
			price:     validPrice,
			quantity:  validQuantity,
			createdAt: createdAt,
			wantErr:   ErrOrderSymbolRequired,
		},
		{
			name:      "symbol with only spaces",
			id:        "ord_001",
			symbol:    "   ",
			side:      SideBuy,
			price:     validPrice,
			quantity:  validQuantity,
			createdAt: createdAt,
			wantErr:   ErrOrderSymbolRequired,
		},
		{
			name:      "invalid side",
			id:        "ord_001",
			symbol:    "BTC-USD",
			side:      Side("HOLD"),
			price:     validPrice,
			quantity:  validQuantity,
			createdAt: createdAt,
			wantErr:   ErrInvalidSide,
		},
		{
			name:      "zero price",
			id:        "ord_001",
			symbol:    "BTC-USD",
			side:      SideBuy,
			price:     Price(0),
			quantity:  validQuantity,
			createdAt: createdAt,
			wantErr:   ErrInvalidPrice,
		},
		{
			name:      "negative price",
			id:        "ord_001",
			symbol:    "BTC-USD",
			side:      SideBuy,
			price:     Price(-1),
			quantity:  validQuantity,
			createdAt: createdAt,
			wantErr:   ErrInvalidPrice,
		},
		{
			name:      "zero order quantity",
			id:        "ord_001",
			symbol:    "BTC-USD",
			side:      SideBuy,
			price:     validPrice,
			quantity:  Quantity(0),
			createdAt: createdAt,
			wantErr:   ErrOrderQuantityInvalid,
		},
		{
			name:      "negative order quantity",
			id:        "ord_001",
			symbol:    "BTC-USD",
			side:      SideBuy,
			price:     validPrice,
			quantity:  Quantity(-1),
			createdAt: createdAt,
			wantErr:   ErrOrderQuantityInvalid,
		},
		{
			name:      "zero creation time",
			id:        "ord_001",
			symbol:    "BTC-USD",
			side:      SideBuy,
			price:     validPrice,
			quantity:  validQuantity,
			createdAt: time.Time{},
			wantErr:   ErrOrderCreatedAtRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order, err := NewOrder(
				tt.id,
				tt.symbol,
				tt.side,
				tt.price,
				tt.quantity,
				tt.createdAt,
			)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			if order != nil {
				t.Errorf("expected nil order, got %#v", order)
			}
		})
	}
}
