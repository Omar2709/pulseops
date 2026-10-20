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
			var total atomic.Int64
			var symbolWait atomic.Int64
			var globalWait atomic.Int64
			var applyTime atomic.Int64
			var publication atomic.Int64
			observer := trading.TimingObserverFunc(func(timing trading.OperationTiming) {
				total.Add(1)
				symbolWait.Add(int64(timing.SymbolLockWait))
				globalWait.Add(int64(timing.GlobalLockWait))
				applyTime.Add(int64(timing.StoreApply))
				publication.Add(int64(timing.Publication))
			})
			service := trading.NewServiceWithStoreAndTimingObserver(store, observer)
			var nextPair atomic.Uint64
			// A matched pair must remain adjacent for its symbol. Without
			// this gate, a BUY could fill another goroutine's SELL, making
			// the benchmark workload nondeterministic.
			pairLocks := make([]sync.Mutex, scenario.symbols)

			submitPair := func() error {
				pair := nextPair.Add(1)
				symbolIndex := (pair - 1) % scenario.symbols
				pairLocks[symbolIndex].Lock()
				defer pairLocks[symbolIndex].Unlock()
				symbol := "ASSET" + strconv.FormatUint(symbolIndex, 10) + "USD"
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
			// Phase averages are nanoseconds per SubmitOrder, not per
			// matched SELL+BUY pair. Observer overhead is included in ns/op.
			if n := total.Load(); n != 0 {
				divisor := float64(n)
				b.ReportMetric(float64(symbolWait.Load())/divisor, "symbol_wait_ns/submit")
				b.ReportMetric(float64(globalWait.Load())/divisor, "global_wait_ns/submit")
				b.ReportMetric(float64(applyTime.Load())/divisor, "store_apply_ns/submit")
				b.ReportMetric(float64(publication.Load())/divisor, "publication_ns/submit")
			}

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

// BenchmarkTradingServicePostgresResting measures durable non-crossing order
// submissions across independent symbols. Live book depth grows with b.N:
// this is a throughput workload, not a constant-depth microbenchmark.
func BenchmarkTradingServicePostgresResting(b *testing.B) {
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

			var total atomic.Int64
			var globalWait atomic.Int64
			var applyTime atomic.Int64
			observer := trading.TimingObserverFunc(func(timing trading.OperationTiming) {
				total.Add(1)
				globalWait.Add(int64(timing.GlobalLockWait))
				applyTime.Add(int64(timing.StoreApply))
			})
			service := trading.NewServiceWithStoreAndTimingObserver(store, observer)
			var nextID atomic.Uint64
			submit := func() error {
				n := nextID.Add(1)
				symbol := "ASSET" + strconv.FormatUint((n-1)%scenario.symbols, 10) + "USD"
				id := "resting-" + strconv.FormatUint(n, 10)
				result, err := service.SubmitOrder(context.Background(), trading.SubmitOrderInput{
					ID:       id,
					Symbol:   symbol,
					Side:     orders.SideBuy,
					Price:    orders.Price(10_000),
					Quantity: orders.Quantity(1_000),
				})
				if err != nil {
					return err
				}
				if result.Replayed || len(result.Trades) != 0 {
					return fmt.Errorf("resting submission %s unexpectedly matched or replayed", id)
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
						if err := submit(); err != nil {
							once.Do(func() { firstErr = err })
						}
					}
				})
				if firstErr != nil {
					b.Fatalf("parallel resting submission: %v", firstErr)
				}
			} else {
				for i := 0; i < b.N; i++ {
					if err := submit(); err != nil {
						b.Fatalf("serial resting submission: %v", err)
					}
				}
			}
			b.StopTimer()
			if count := total.Load(); count != 0 {
				b.ReportMetric(float64(globalWait.Load())/float64(count), "global_wait_ns/submit")
				b.ReportMetric(float64(applyTime.Load())/float64(count), "store_apply_ns/submit")
			}

			// Exclude verification from timing; each submission must
			// persist one order without advancing the trade counter.
			var count int
			if err := pool.QueryRow(context.Background(),
				"SELECT COUNT(*) FROM orders",
			).Scan(&count); err != nil {
				b.Fatal(err)
			}
			if count != b.N {
				b.Fatalf("expected %d durable resting orders, got %d", b.N, count)
			}
			var tradeSequence int64
			if err := pool.QueryRow(context.Background(),
				"SELECT trade_sequence FROM matching_engine_state WHERE id = 1",
			).Scan(&tradeSequence); err != nil {
				b.Fatal(err)
			}
			if tradeSequence != 0 {
				b.Fatalf("resting submissions advanced trade sequence to %d", tradeSequence)
			}
		})
	}
}


// BenchmarkTradingServicePostgresCancel measures submit+cancel pairs on a
// real PostgreSQL store. Matching is excluded; every order remains resting
// until cancellation. The benchmark checks all rows were durably cancelled.
func BenchmarkTradingServicePostgresCancel(b *testing.B) {
	for _, scenario := range []struct {
		name     string
		symbols  uint64
		parallel bool
	}{
		{name: "serial", symbols: 1},
		{name: "parallel_1_symbol", symbols: 1, parallel: true},
		{name: "parallel_8_symbols", symbols: 8, parallel: true},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			pool := benchmarkPostgresPool(b)
			store, err := NewStore(pool)
			if err != nil {
				b.Fatal(err)
			}
			var observed atomic.Int64
			var globalWait atomic.Int64
			var applyTime atomic.Int64
			observer := trading.TimingObserverFunc(func(timing trading.OperationTiming) {
				if timing.Operation != trading.TimingCancel {
					return
				}
				observed.Add(1)
				globalWait.Add(int64(timing.GlobalLockWait))
				applyTime.Add(int64(timing.StoreApply))
			})
			service := trading.NewServiceWithStoreAndTimingObserver(store, observer)
			var nextID atomic.Uint64
			submitAndCancel := func() error {
				n := nextID.Add(1)
				id := "cancellable-" + strconv.FormatUint(n, 10)
				symbol := "ASSET" + strconv.FormatUint((n-1)%scenario.symbols, 10) + "USD"
				_, err := service.SubmitOrder(context.Background(), trading.SubmitOrderInput{
					ID:       id,
					Symbol:   symbol,
					Side:     orders.SideBuy,
					Price:    orders.Price(10_000),
					Quantity: orders.Quantity(1_000),
				})
				if err != nil {
					return fmt.Errorf("submit %s: %w", id, err)
				}
				cancelled, err := service.CancelOrder(context.Background(), id)
				if err != nil {
					return fmt.Errorf("cancel %s: %w", id, err)
				}
				if cancelled.Status != orders.OrderStatusCancelled {
					return fmt.Errorf("cancel %s: got %s", id, cancelled.Status)
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
						if err := submitAndCancel(); err != nil {
							once.Do(func() { firstErr = err })
						}
					}
				})
				if firstErr != nil {
					b.Fatalf("parallel submissions and cancellations: %v", firstErr)
				}
			} else {
				for i := 0; i < b.N; i++ {
					if err := submitAndCancel(); err != nil {
						b.Fatalf("submissions and cancellations: %v", err)
					}
				}
			}
			b.StopTimer()
			if n := observed.Load(); n != 0 {
				b.ReportMetric(float64(globalWait.Load())/float64(n), "cancel_global_wait_ns/op")
				b.ReportMetric(float64(applyTime.Load())/float64(n), "cancel_store_apply_ns/op")
			}
			var total, cancelled, trades int
			if err := pool.QueryRow(context.Background(),
				"SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'CANCELLED') FROM orders",
			).Scan(&total, &cancelled); err != nil {
				b.Fatal(err)
			}
			if err := pool.QueryRow(context.Background(),
				"SELECT COUNT(*) FROM trades",
			).Scan(&trades); err != nil {
				b.Fatal(err)
			}
			if total != b.N || cancelled != b.N || trades != 0 {
				b.Fatalf("expected %d cancelled, 0 trades: total=%d cancelled=%d trades=%d",
					b.N, total, cancelled, trades)
			}
		})
	}
}
