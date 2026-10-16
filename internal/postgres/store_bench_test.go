package postgres

import (
	"context"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This benchmark uses the same PostgreSQL service as integration tests.
// Unlike in-memory microbenchmarks, it includes a real transaction, SQL
// statements, connection-pool behavior, and commit latency.
func benchmarkPostgresPool(b *testing.B) *pgxpool.Pool {
	b.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		b.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := Open(ctx, databaseURL)
	if err != nil {
		b.Fatalf("open benchmark PostgreSQL: %v", err)
	}
	b.Cleanup(pool.Close)

	if err := Migrate(ctx, pool); err != nil {
		b.Fatalf("apply benchmark migrations: %v", err)
	}
	// All setup is excluded from the measured interval. Each sub-benchmark
	// starts with the same empty tables and global trade sequence.
	if _, err := pool.Exec(ctx, "TRUNCATE TABLE trades, orders"); err != nil {
		b.Fatalf("truncate benchmark tables: %v", err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE matching_engine_state SET trade_sequence = 0 WHERE id = 1",
	); err != nil {
		b.Fatalf("reset trade sequence: %v", err)
	}

	return pool
}

func BenchmarkPostgresApplyResting(b *testing.B) {
	for _, scenario := range []struct {
		name     string
		parallel bool
	}{
		{name: "serial"},
		{name: "parallel", parallel: true},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			pool := benchmarkPostgresPool(b)
			store, err := NewStore(pool)
			if err != nil {
				b.Fatal(err)
			}

			var sequence atomic.Uint64
			at := time.Date(2026, time.October, 8, 0, 0, 0, 0, time.UTC)
			ctx := context.Background()

			insert := func() error {
				n := sequence.Add(1)
				id := "bench-order-" + strconv.FormatUint(n, 10)
				return store.Apply(ctx, trading.StateChange{
					Orders: []trading.OrderSnapshot{{
						ID:                id,
						Symbol:            "BTCUSD",
						Side:              orders.SideBuy,
						Price:             orders.Price(10_000),
						Quantity:          orders.Quantity(1_000),
						RemainingQuantity: orders.Quantity(1_000),
						Status:            orders.OrderStatusOpen,
						CreatedAt:         at,
						UpdatedAt:         at,
					}},
					BookSequences: map[string]uint64{id: n - 1},
					TradeSequence: 0,
				})
			}

			b.ReportAllocs()
			b.ResetTimer()
			if scenario.parallel {
				var once sync.Once
				var firstErr error
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						if err := insert(); err != nil {
							once.Do(func() { firstErr = err })
						}
					}
				})
				if firstErr != nil {
					b.Fatalf("parallel PostgreSQL apply: %v", firstErr)
				}
			} else {
				for i := 0; i < b.N; i++ {
					if err := insert(); err != nil {
						b.Fatalf("serial PostgreSQL apply: %v", err)
					}
				}
			}
		})
	}
}
