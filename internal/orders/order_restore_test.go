package orders

import (
	"errors"
	"testing"
	"time"
)

func TestRestoreOrderRebuildsLifecycleStates(t *testing.T) {
	createdAt := time.Date(
		2026,
		time.October,
		8,
		4,
		0,
		0,
		0,
		time.UTC,
	)
	updatedAt := createdAt.Add(time.Second)

	tests := []struct {
		name    string
		filled  Quantity
		status  OrderStatus
		updated time.Time
	}{
		{name: "pending", status: OrderStatusPending, updated: createdAt},
		{name: "open", status: OrderStatusOpen, updated: updatedAt},
		{
			name: "partial", filled: Quantity(2_000_000),
			status: OrderStatusPartiallyFilled, updated: updatedAt,
		},
		{
			name: "filled", filled: Quantity(5_000_000),
			status: OrderStatusFilled, updated: updatedAt,
		},
		{
			name: "cancelled", filled: Quantity(2_000_000),
			status: OrderStatusCancelled, updated: updatedAt,
		},
		{name: "rejected", status: OrderStatusRejected, updated: updatedAt},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			order, err := RestoreOrder(
				"order-001",
				"BTCUSD",
				SideBuy,
				Price(6_000_000),
				Quantity(5_000_000),
				test.filled,
				test.status,
				createdAt,
				test.updated,
			)
			if err != nil {
				t.Fatalf("restoring order: %v", err)
			}

			if order.Status() != test.status {
				t.Errorf(
					"expected status %s, got %s",
					test.status,
					order.Status(),
				)
			}

			if order.FilledQuantity() != test.filled {
				t.Errorf(
					"expected filled %d, got %d",
					test.filled,
					order.FilledQuantity(),
				)
			}

			if !order.UpdatedAt().Equal(test.updated) {
				t.Errorf(
					"expected updated_at %v, got %v",
					test.updated,
					order.UpdatedAt(),
				)
			}
		})
	}
}

func TestRestoreOrderRejectsImpossibleState(t *testing.T) {
	at := time.Date(
		2026,
		time.October,
		8,
		4,
		0,
		0,
		0,
		time.UTC,
	)

	_, err := RestoreOrder(
		"order-001",
		"BTCUSD",
		SideBuy,
		Price(6_000_000),
		Quantity(5_000_000),
		Quantity(5_000_000),
		OrderStatusPartiallyFilled,
		at,
		at.Add(time.Second),
	)

	if !errors.Is(err, ErrOrderRestoreStateInvalid) {
		t.Fatalf(
			"expected ErrOrderRestoreStateInvalid, got %v",
			err,
		)
	}
}
