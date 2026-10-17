package postgres

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
)

// BenchmarkTradingServicePostgresMatch measures a complete matched pair:
// SubmitOrder(SELL) + SubmitOrder(BUY), including book staging, the global
// service commit mutex, two real PostgreSQL transactions, and one trade.
// It intentionally does not exercise the HTTP transport.
func BenchmarkTradingServicePostgresMatch(b *testing.B) {
	scenarios := []struct {
		name     string
		symbols  uint64
		parallel bool
	}{
		{name: "serial", symbols: 1},
		{name: "parallel_1_symbol", symbols: 1, parallel: true},
		{name: "parallel_8_symbols", symbols: 8, parallel: true},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			pool := benchmarkPostgresPool(b)
			store, err := NewStore(pool)
			if err != nil {
				b.Fatal(err)
			}
			service := trading.NewServiceWithStore(store)
			var nextPair atomic.Uint64

			submitPair := func() error {
				pair := nextPair.Add(1)
				symbol := "ASSET" + strconv.FormatUint((pair-1)%scenario.symbols, 10) + "USD"
				suffix := strconv.FormatUint(pair, 10)
				sellID := "sell-" + suffix
				buyID := "buy-" + suffix
				ctx := context.Background()
				sell := trading.SubmitOrderInput{
					ID:       sellID,
					Symbol:   symbol,
					Side:     orders.SideSell,
					Price:    orders.Price(10_000),
					Quantity: orders.Quantity(1_000),
				}
				if _, err := service.SubmitOrder(ctx, sell); err != nil {
					return fmt.Errorf("resting sell %s: %w", sellID, err)
				}
				buy := trading.SubmitOrderInput{
					ID:       buyID,
					Symbol:   symbol,
					Side:     orders.SideBuy,
					Price:    orders.Price(10_000),
					Quantity: orders.Quantity(1_000),
				}
				result, err := service.SubmitOrder(ctx, buy)
				if err != nil {
					return fmt.Errorf("matching buy %s: %w", buyID, err)
				}
				if len(result.Trades) != 1 ||
					result.Trades[0].BuyOrderID() != buyID ||
					result.Trades[0].SellOrderID() != sellID {
					return fmt.Errorf("pair %d: unexpected trades %+v", pair, result.Trades)
				}
				return nil
			}

			b.ReportAllocs()
			b.ResetTimer()
			if scenario.parallel {
				var once sync.Once
				var firstErr error
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						if err := submitPair(); err != nil {
							once.Do(func() { firstErr = err })
						}
					}
				})
				if firstErr != nil {
					b.Fatalf("parallel service submission: %v", firstErr)
				}
			} else {
				for i := 0; i < b.N; i++ {
					if err := submitPair(); err != nil {
						b.Fatalf("serial service submission: %v", err)
					}
				}
			}
			b.StopTimer()

			// This check is excluded from the measurement. Every pair must
			// persist exactly one trade with no gaps in the global sequence.
			var tradeCount int
			if err := pool.QueryRow(context.Background(),
				"SELECT COUNT(*) FROM trades",
			).Scan(&tradeCount); err != nil {
				b.Fatalf("count trades: %v", err)
			}
			if tradeCount != b.N {
				b.Fatalf("expected %d committed trades, got %d", b.N, tradeCount)
			}
			var sequence int64
			if err := pool.QueryRow(context.Background(),
				"SELECT trade_sequence FROM matching_engine_state WHERE id = 1",
			).Scan(&sequence); err != nil {
				b.Fatalf("read global trade sequence: %v", err)
			}
			if sequence != int64(b.N) {
				b.Fatalf("expected durable sequence %d, got %d", b.N, sequence)
			}
		})
	}
}
