package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
)

func TestOrderIDReplaySurvivesPostgreSQLRecovery(t *testing.T) {
	pool := testPostgresPool(t)
	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("creating state store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	service := trading.NewServiceWithStore(store)
	input := trading.SubmitOrderInput{
		ID:       "buy-retry",
		Symbol:   "BTCUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(6_100_000),
		Quantity: orders.Quantity(1_000_000),
	}
	first, err := service.SubmitOrder(ctx, input)
	if err != nil {
		t.Fatalf("submitting order: %v", err)
	}
	if first.Replayed {
		t.Error("first order unexpectedly marked as replay")
	}

	state, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	recovered, err := trading.NewRecoveredService(store, state)
	if err != nil {
		t.Fatalf("recovering service: %v", err)
	}

	input.Symbol = "btcusd"
	replay, err := recovered.SubmitOrder(ctx, input)
	if err != nil {
		t.Fatalf("replaying order after restart: %v", err)
	}
	if !replay.Replayed || len(replay.Trades) != 0 {
		t.Errorf("expected replay without trades: %+v", replay)
	}
	if replay.Order.ID != first.Order.ID || replay.Order.Status != first.Order.Status {
		t.Errorf("unexpected replay snapshot: %+v", replay.Order)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM orders").Scan(&count); err != nil {
		t.Fatalf("counting orders: %v", err)
	}
	if count != 1 {
		t.Errorf("retry added an order: got %d rows", count)
	}
}
