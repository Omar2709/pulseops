package trading

import (
	"context"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

func TestRecoveredServicePreservesFIFOAndTradeSequence(
	t *testing.T,
) {
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
	now := at.Add(time.Hour)

	sequenceNine := uint64(9)
	sequenceThree := uint64(3)

	state := RecoveryState{
		Orders: []RecoveredOrder{
			{
				Order: OrderSnapshot{
					ID:                "sell-later",
					Symbol:            "BTCUSD",
					Side:              orders.SideSell,
					Price:             orders.Price(6_000_000),
					Quantity:          orders.Quantity(1_000_000),
					FilledQuantity:    0,
					RemainingQuantity: orders.Quantity(1_000_000),
					Status:            orders.OrderStatusOpen,
					CreatedAt:         at,
					UpdatedAt:         at,
				},
				BookSequence: &sequenceNine,
			},
			{
				Order: OrderSnapshot{
					ID:                "sell-priority",
					Symbol:            "BTCUSD",
					Side:              orders.SideSell,
					Price:             orders.Price(6_000_000),
					Quantity:          orders.Quantity(1_000_000),
					FilledQuantity:    0,
					RemainingQuantity: orders.Quantity(1_000_000),
					Status:            orders.OrderStatusOpen,
					CreatedAt:         at.Add(time.Second),
					UpdatedAt:         at.Add(time.Second),
				},
				BookSequence: &sequenceThree,
			},
			{
				Order: OrderSnapshot{
					ID:                "filled-old",
					Symbol:            "ETHUSD",
					Side:              orders.SideBuy,
					Price:             orders.Price(300_000),
					Quantity:          orders.Quantity(2_000_000),
					FilledQuantity:    orders.Quantity(2_000_000),
					RemainingQuantity: 0,
					Status:            orders.OrderStatusFilled,
					CreatedAt:         at,
					UpdatedAt:         at.Add(time.Second),
				},
			},
		},
		TradeSequence: 41,
	}

	service, err := newRecoveredService(
		func() time.Time {
			return now
		},
		noopStateStore{},
		state,
	)
	if err != nil {
		t.Fatalf("recovering service: %v", err)
	}

	historical, err := service.GetOrder("filled-old")
	if err != nil {
		t.Fatalf("getting recovered terminal order: %v", err)
	}

	if historical.Status != orders.OrderStatusFilled {
		t.Errorf(
			"expected FILLED history, got %s",
			historical.Status,
		)
	}

	result, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "buy-new",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(1_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting after recovery: %v", err)
	}

	if len(result.Trades) != 1 {
		t.Fatalf(
			"expected 1 trade, got %d",
			len(result.Trades),
		)
	}

	trade := result.Trades[0]

	if trade.ID() != "trade-000042" {
		t.Errorf(
			"expected trade-000042, got %q",
			trade.ID(),
		)
	}

	if trade.SellOrderID() != "sell-priority" {
		t.Errorf(
			"expected recovered FIFO order sell-priority, got %q",
			trade.SellOrderID(),
		)
	}
}
