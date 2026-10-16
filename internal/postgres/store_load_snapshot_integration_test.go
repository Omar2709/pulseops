package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
	"github.com/jackc/pgx/v5"
)

// sequenceInterceptTx commits another transaction after the order query has
// finished but before recovery reads the matching-engine sequence.
type sequenceInterceptTx struct {
	pgx.Tx
	beforeSequence func()
}

func (tx *sequenceInterceptTx) QueryRow(
	ctx context.Context,
	sql string,
	args ...any,
) pgx.Row {
	tx.beforeSequence()
	return tx.Tx.QueryRow(ctx, sql, args...)
}

func TestLoadRecoverySnapshotConsistentAcrossConcurrentCommit(t *testing.T) {
	pool := testPostgresPool(t)

	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	at := time.Date(2026, time.October, 8, 10, 0, 0, 0, time.UTC)
	sell := trading.OrderSnapshot{
		ID:                "sell-resting",
		Symbol:            "BTCUSD",
		Side:              orders.SideSell,
		Price:             orders.Price(6_000_000),
		Quantity:          orders.Quantity(1_000_000),
		RemainingQuantity: orders.Quantity(1_000_000),
		Status:            orders.OrderStatusOpen,
		CreatedAt:         at,
		UpdatedAt:         at,
	}

	if err := store.Apply(ctx, trading.StateChange{
		Orders:        []trading.OrderSnapshot{sell},
		BookSequences: map[string]uint64{sell.ID: 0},
	}); err != nil {
		t.Fatalf("seeding resting order: %v", err)
	}

	executedAt := at.Add(time.Second)
	filledSell := sell
	filledSell.FilledQuantity = sell.Quantity
	filledSell.RemainingQuantity = 0
	filledSell.Status = orders.OrderStatusFilled
	filledSell.UpdatedAt = executedAt

	buy := trading.OrderSnapshot{
		ID:             "buy-incoming",
		Symbol:         "BTCUSD",
		Side:           orders.SideBuy,
		Price:          orders.Price(6_100_000),
		Quantity:       orders.Quantity(1_000_000),
		FilledQuantity: orders.Quantity(1_000_000),
		Status:         orders.OrderStatusFilled,
		CreatedAt:      executedAt,
		UpdatedAt:      executedAt,
	}
	trade, err := orders.NewTrade(
		"trade-000001",
		"BTCUSD",
		buy.ID,
		sell.ID,
		sell.Price,
		sell.Quantity,
		executedAt,
	)
	if err != nil {
		t.Fatalf("creating trade: %v", err)
	}

	// REPEATABLE READ pins the order and trade-sequence queries to the
	// same database snapshot even when an Apply commits between them.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		t.Fatalf("starting recovery transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	var writeErr error
	intercept := &sequenceInterceptTx{
		Tx: tx,
		beforeSequence: func() {
			writeErr = store.Apply(ctx, trading.StateChange{
				Orders:        []trading.OrderSnapshot{filledSell, buy},
				Trades:        []*orders.Trade{trade},
				TradeSequence: 1,
			})
		},
	}

	recovered, err := loadRecoverySnapshot(ctx, intercept)
	if writeErr != nil {
		t.Fatalf("committing a matching trade during recovery: %v", writeErr)
	}
	if err != nil {
		t.Fatalf("loading consistent snapshot: %v", err)
	}

	if recovered.TradeSequence != 0 {
		t.Errorf("expected pre-commit trade sequence 0, got %d", recovered.TradeSequence)
	}
	if len(recovered.Orders) != 1 {
		t.Fatalf("expected one pre-commit order, got %d", len(recovered.Orders))
	}
	if recovered.Orders[0].Order.Status != orders.OrderStatusOpen {
		t.Errorf("expected pre-commit OPEN order, got %s", recovered.Orders[0].Order.Status)
	}
	if recovered.Orders[0].BookSequence == nil || *recovered.Orders[0].BookSequence != 0 {
		t.Error("pre-commit order did not retain its book sequence")
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing read-only transaction: %v", err)
	}

	latest, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("loading after matching trade: %v", err)
	}
	if latest.TradeSequence != 1 || len(latest.Orders) != 2 {
		t.Errorf("expected post-commit state with two orders and sequence 1, got %+v", latest)
	}
}
