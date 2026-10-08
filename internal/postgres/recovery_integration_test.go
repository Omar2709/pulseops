package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
)

func TestPostgreSQLRecoveryPreservesFIFOAndTradeSequence(
	t *testing.T,
) {
	pool := testPostgresPool(t)

	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	at := time.Date(
		2020,
		time.January,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)

	sellLater := trading.OrderSnapshot{
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
	}

	sellPriority := trading.OrderSnapshot{
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
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := store.Apply(
		ctx,
		trading.StateChange{
			Orders: []trading.OrderSnapshot{
				sellLater,
				sellPriority,
			},
			BookSequences: map[string]uint64{
				"sell-later":    9,
				"sell-priority": 3,
			},
			TradeSequence: 41,
		},
	); err != nil {
		t.Fatalf("seeding recoverable state: %v", err)
	}

	state, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("loading recovery state: %v", err)
	}

	service, err := trading.NewRecoveredService(
		store,
		state,
	)
	if err != nil {
		t.Fatalf("rebuilding service: %v", err)
	}

	result, err := service.SubmitOrder(
		ctx,
		trading.SubmitOrderInput{
			ID:       "buy-after-restart",
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
			"expected FIFO sell-priority, got %q",
			trade.SellOrderID(),
		)
	}

	reloaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("reloading persisted state: %v", err)
	}

	if reloaded.TradeSequence != 42 {
		t.Errorf(
			"expected recovered sequence 42, got %d",
			reloaded.TradeSequence,
		)
	}

	var remainingSequence *uint64

	for _, recovered := range reloaded.Orders {
		if recovered.Order.ID == "sell-later" {
			remainingSequence = recovered.BookSequence
		}

		if recovered.Order.ID == "sell-priority" &&
			recovered.BookSequence != nil {
			t.Error("filled recovered order kept an active book sequence")
		}
	}

	if remainingSequence == nil ||
		*remainingSequence != 9 {
		t.Fatalf(
			"expected sell-later to keep sequence 9",
		)
	}
}
