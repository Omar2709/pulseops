package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
)

func TestRestingSubmissionAndCancellationDoNotWaitForTradeSequenceLock(t *testing.T) {
	pool := testPostgresPool(t)
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	service := trading.NewServiceWithStore(store)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	submitSell := func(id string) {
		t.Helper()
		if _, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
			ID:       id,
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(1_000_000),
		}); err != nil {
			t.Fatalf("submit resting %s: %v", id, err)
		}
	}
	submitSell("sell-first")

	// Hold a PostgreSQL row lock from a separate connection. Resting
	// submissions and cancellation must not need this global row.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocking transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	var sequence int64
	if err := tx.QueryRow(ctx,
		"SELECT trade_sequence FROM matching_engine_state WHERE id = 1 FOR UPDATE",
	).Scan(&sequence); err != nil {
		t.Fatalf("lock matching engine sequence: %v", err)
	}
	if sequence != 0 {
		t.Fatalf("expected initial sequence 0, got %d", sequence)
	}

	// A regression that updates matching_engine_state on every write
	// would block until the short request deadline and fail this test.
	requestCtx, requestCancel := context.WithTimeout(ctx, 2*time.Second)
	defer requestCancel()
	if _, err := service.SubmitOrder(requestCtx, trading.SubmitOrderInput{
		ID:       "sell-second",
		Symbol:   "BTCUSD",
		Side:     orders.SideSell,
		Price:    orders.Price(6_000_000),
		Quantity: orders.Quantity(1_000_000),
	}); err != nil {
		t.Fatalf("resting submission waited for global trade sequence lock: %v", err)
	}
	cancelled, err := service.CancelOrder(requestCtx, "sell-second")
	if err != nil {
		t.Fatalf("cancellation waited for global trade sequence lock: %v", err)
	}
	if cancelled.Status != orders.OrderStatusCancelled {
		t.Fatalf("unexpected cancellation status %s", cancelled.Status)
	}

	// Trade-producing submissions must still advance the sequence.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("release global sequence lock: %v", err)
	}

	buy, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
		ID:       "buy-matching",
		Symbol:   "BTCUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(6_000_000),
		Quantity: orders.Quantity(1_000_000),
	})
	if err != nil {
		t.Fatalf("matching submission: %v", err)
	}
	if len(buy.Trades) != 1 || buy.Trades[0].ID() != "trade-000001" ||
		buy.Trades[0].SellOrderID() != "sell-first" {
		t.Fatalf("unexpected matching result after unlocked sequence: %+v", buy.Trades)
	}

	state, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.TradeSequence != 1 || len(state.Orders) != 3 {
		t.Errorf("unexpected durable state: sequence %d, orders %d",
			state.TradeSequence, len(state.Orders))
	}
}

func TestSkipTradeSequenceUpdateRejectsNewTrades(t *testing.T) {
	store := &Store{}
	err := store.Apply(context.Background(), trading.StateChange{
		Trades:                  []*orders.Trade{nil},
		SkipTradeSequenceUpdate: true,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot skip trade sequence") {
		t.Fatalf("expected invalid optimization flag to be rejected, got %v", err)
	}
}
