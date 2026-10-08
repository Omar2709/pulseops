package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	pool, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("opening test PostgreSQL: %v", err)
	}

	t.Cleanup(pool.Close)

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	if _, err := pool.Exec(
		ctx,
		"TRUNCATE TABLE trades, orders",
	); err != nil {
		t.Fatalf("cleaning test tables: %v", err)
	}

	if _, err := pool.Exec(
		ctx,
		`
			UPDATE matching_engine_state
			SET trade_sequence = 0
			WHERE id = 1
		`,
	); err != nil {
		t.Fatalf("resetting matching engine state: %v", err)
	}

	return pool
}

func TestMigrateIsIdempotent(t *testing.T) {
	pool := testPostgresPool(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("running migrations again: %v", err)
	}

	var count int

	if err := pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&count); err != nil {
		t.Fatalf("counting migrations: %v", err)
	}

	if count != 6 {
		t.Errorf(
			"expected 6 applied migrations, got %d",
			count,
		)
	}
}

func TestStoreApplyPersistsOrdersAndTrades(
	t *testing.T,
) {
	pool := testPostgresPool(t)

	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	at := time.Date(
		2026,
		time.October,
		8,
		3,
		0,
		0,
		123456789,
		time.UTC,
	)

	sell := trading.OrderSnapshot{
		ID:                "sell-001",
		Symbol:            "BTCUSD",
		Side:              orders.SideSell,
		Price:             orders.Price(6_000_000),
		Quantity:          orders.Quantity(5_000_000),
		FilledQuantity:    orders.Quantity(5_000_000),
		RemainingQuantity: 0,
		Status:            orders.OrderStatusFilled,
		CreatedAt:         at,
		UpdatedAt:         at.Add(time.Second),
	}

	buy := trading.OrderSnapshot{
		ID:                "buy-001",
		Symbol:            "BTCUSD",
		Side:              orders.SideBuy,
		Price:             orders.Price(6_100_000),
		Quantity:          orders.Quantity(5_000_000),
		FilledQuantity:    orders.Quantity(5_000_000),
		RemainingQuantity: 0,
		Status:            orders.OrderStatusFilled,
		CreatedAt:         at.Add(time.Second),
		UpdatedAt:         at.Add(time.Second),
	}

	trade, err := orders.NewTrade(
		"trade-000001",
		"BTCUSD",
		"buy-001",
		"sell-001",
		orders.Price(6_000_000),
		orders.Quantity(5_000_000),
		at.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("creating trade: %v", err)
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
				sell,
				buy,
			},
			Trades:        []*orders.Trade{trade},
			TradeSequence: 1,
		},
	); err != nil {
		t.Fatalf("persisting state change: %v", err)
	}

	var orderCount int
	var tradeCount int

	if err := pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM orders",
	).Scan(&orderCount); err != nil {
		t.Fatalf("counting orders: %v", err)
	}

	if err := pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM trades",
	).Scan(&tradeCount); err != nil {
		t.Fatalf("counting trades: %v", err)
	}

	if orderCount != 2 {
		t.Errorf("expected 2 orders, got %d", orderCount)
	}

	if tradeCount != 1 {
		t.Errorf("expected 1 trade, got %d", tradeCount)
	}

	cancelled := buy
	cancelled.FilledQuantity = orders.Quantity(2_000_000)
	cancelled.RemainingQuantity = orders.Quantity(3_000_000)
	cancelled.Status = orders.OrderStatusCancelled
	cancelled.UpdatedAt = at.Add(2 * time.Second)

	if err := store.Apply(
		ctx,
		trading.StateChange{
			Orders:        []trading.OrderSnapshot{cancelled},
			TradeSequence: 1,
		},
	); err != nil {
		t.Fatalf("persisting cancellation: %v", err)
	}

	var status string
	var filled int64

	if err := pool.QueryRow(
		ctx,
		`
			SELECT status, filled_units
			FROM orders
			WHERE id = 'buy-001'
		`,
	).Scan(&status, &filled); err != nil {
		t.Fatalf("reading cancelled order: %v", err)
	}

	if status != "CANCELLED" {
		t.Errorf("expected CANCELLED, got %q", status)
	}

	if filled != 2_000_000 {
		t.Errorf("expected 2000000 filled units, got %d", filled)
	}
}

func TestStoreApplyRollsBackWholeStateChange(
	t *testing.T,
) {
	pool := testPostgresPool(t)

	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

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

	sell := trading.OrderSnapshot{
		ID:                "sell-001",
		Symbol:            "BTCUSD",
		Side:              orders.SideSell,
		Price:             orders.Price(6_000_000),
		Quantity:          orders.Quantity(1_000_000),
		FilledQuantity:    orders.Quantity(1_000_000),
		RemainingQuantity: 0,
		Status:            orders.OrderStatusFilled,
		CreatedAt:         at,
		UpdatedAt:         at,
	}

	buy := trading.OrderSnapshot{
		ID:                "buy-001",
		Symbol:            "BTCUSD",
		Side:              orders.SideBuy,
		Price:             orders.Price(6_000_000),
		Quantity:          orders.Quantity(1_000_000),
		FilledQuantity:    orders.Quantity(1_000_000),
		RemainingQuantity: 0,
		Status:            orders.OrderStatusFilled,
		CreatedAt:         at,
		UpdatedAt:         at,
	}

	trade, err := orders.NewTrade(
		"trade-000001",
		"BTCUSD",
		"buy-001",
		"sell-001",
		orders.Price(6_000_000),
		orders.Quantity(1_000_000),
		at,
	)
	if err != nil {
		t.Fatalf("creating trade: %v", err)
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
				sell,
				buy,
			},
			Trades:        []*orders.Trade{trade},
			TradeSequence: 1,
		},
	); err != nil {
		t.Fatalf("seeding persisted trade: %v", err)
	}

	extra := trading.OrderSnapshot{
		ID:                "buy-extra",
		Symbol:            "BTCUSD",
		Side:              orders.SideBuy,
		Price:             orders.Price(5_900_000),
		Quantity:          orders.Quantity(1_000_000),
		FilledQuantity:    0,
		RemainingQuantity: orders.Quantity(1_000_000),
		Status:            orders.OrderStatusOpen,
		CreatedAt:         at.Add(time.Second),
		UpdatedAt:         at.Add(time.Second),
	}

	err = store.Apply(
		ctx,
		trading.StateChange{
			Orders: []trading.OrderSnapshot{extra},
			Trades: []*orders.Trade{trade},
			BookSequences: map[string]uint64{
				"buy-extra": 0,
			},
			TradeSequence: 1,
		},
	)
	if err == nil {
		t.Fatal("expected duplicate trade persistence to fail")
	}

	var exists bool

	if err := pool.QueryRow(
		ctx,
		`
			SELECT EXISTS (
				SELECT 1
				FROM orders
				WHERE id = 'buy-extra'
			)
		`,
	).Scan(&exists); err != nil {
		t.Fatalf("checking rolled-back order: %v", err)
	}

	if exists {
		t.Error("order persisted despite transaction rollback")
	}
}

func TestStoreRejectsImmutableOrderConflict(
	t *testing.T,
) {
	pool := testPostgresPool(t)

	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	at := time.Date(
		2026,
		time.October,
		8,
		5,
		0,
		0,
		0,
		time.UTC,
	)

	original := trading.OrderSnapshot{
		ID:                "buy-001",
		Symbol:            "BTCUSD",
		Side:              orders.SideBuy,
		Price:             orders.Price(6_000_000),
		Quantity:          orders.Quantity(1_000_000),
		FilledQuantity:    0,
		RemainingQuantity: orders.Quantity(1_000_000),
		Status:            orders.OrderStatusOpen,
		CreatedAt:         at,
		UpdatedAt:         at,
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := store.Apply(
		ctx,
		trading.StateChange{
			Orders: []trading.OrderSnapshot{original},
			BookSequences: map[string]uint64{
				"buy-001": 0,
			},
		},
	); err != nil {
		t.Fatalf("persisting original order: %v", err)
	}

	conflict := original
	conflict.Price = orders.Price(6_100_000)

	if err := store.Apply(
		ctx,
		trading.StateChange{
			Orders: []trading.OrderSnapshot{conflict},
			BookSequences: map[string]uint64{
				"buy-001": 0,
			},
		},
	); err == nil {
		t.Fatal("expected immutable order conflict")
	}

	var price int64

	if err := pool.QueryRow(
		ctx,
		`
			SELECT price_units
			FROM orders
			WHERE id = 'buy-001'
		`,
	).Scan(&price); err != nil {
		t.Fatalf("reading original order: %v", err)
	}

	if price != 6_000_000 {
		t.Errorf("immutable price changed to %d", price)
	}
}
