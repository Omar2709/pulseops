package trading

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

var benchmarkOrderSnapshot OrderSnapshot

// Construct the starting depth outside benchmark timing so that the
// measured submission includes only clone, matching, commit, and publication.
func benchmarkServiceWithRestingBids(b *testing.B, depth int) *Service {
	b.Helper()

	at := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	service := newService(func() time.Time { return at })
	book := orders.NewOrderBook()

	for i := 0; i < depth; i++ {
		order, err := orders.NewOrder(
			fmt.Sprintf("resting-%06d", i),
			"BTCUSD",
			orders.SideBuy,
			orders.Price(10_000),
			orders.Quantity(1_000),
			at,
		)
		if err != nil {
			b.Fatalf("construct resting order: %v", err)
		}
		if err := order.Open(at); err != nil {
			b.Fatalf("open resting order: %v", err)
		}
		if err := book.Add(order); err != nil {
			b.Fatalf("add resting order: %v", err)
		}
		service.orders[order.ID()] = order
	}

	if depth > 0 {
		service.books["BTCUSD"] = book
	}
	return service
}

// A fresh service is built outside the measured interval on each iteration;
// the measured request adds one non-crossing BUY to a book of fixed depth.
func BenchmarkSubmitOrderBookDepth(b *testing.B) {
	for _, depth := range []int{0, 100, 1000} {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				service := benchmarkServiceWithRestingBids(b, depth)
				b.StartTimer()

				result, err := service.SubmitOrder(
					context.Background(),
					SubmitOrderInput{
						ID:       "incoming-buy",
						Symbol:   "BTCUSD",
						Side:     orders.SideBuy,
						Price:    orders.Price(10_000),
						Quantity: orders.Quantity(1_000),
					},
				)

				b.StopTimer()
				if err != nil {
					b.Fatalf("submit at depth %d: %v", depth, err)
				}
				if result.Replayed || len(result.Trades) != 0 {
					b.Fatalf("unexpected result at depth %d: %+v", depth, result)
				}
				benchmarkOrderSnapshot = result.Order
			}
		})
	}
}

// Pair each SELL with a crossing BUY to keep active-book depth bounded.
// RunParallel exercises global commit contention at different symbol counts.
// These measurements use the in-memory store, not PostgreSQL throughput.
func BenchmarkSubmitMatchingParallel(b *testing.B) {
	for _, symbols := range []int{1, 8} {
		b.Run(fmt.Sprintf("symbols_%d", symbols), func(b *testing.B) {
			service := NewService()
			names := make([]string, symbols)
			for i := range names {
				names[i] = fmt.Sprintf("ASSET%dUSD", i)
			}

			var serial atomic.Uint64
			var once sync.Once
			var firstErr error

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					sequence := serial.Add(1)
					symbol := names[int((sequence-1)%uint64(symbols))]
					suffix := strconv.FormatUint(sequence, 10)

					_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
						ID:       "sell-" + suffix,
						Symbol:   symbol,
						Side:     orders.SideSell,
						Price:    orders.Price(10_000),
						Quantity: orders.Quantity(1_000),
					})
					if err != nil {
						once.Do(func() { firstErr = err })
						continue
					}
					_, err = service.SubmitOrder(context.Background(), SubmitOrderInput{
						ID:       "buy-" + suffix,
						Symbol:   symbol,
						Side:     orders.SideBuy,
						Price:    orders.Price(10_000),
						Quantity: orders.Quantity(1_000),
					})
					if err != nil {
						once.Do(func() { firstErr = err })
					}
				}
			})

			if firstErr != nil {
				b.Fatalf("concurrent matching: %v", firstErr)
			}
		})
	}
}
