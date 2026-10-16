package orders

import (
	"fmt"
	"testing"
	"time"
)

var (
	benchmarkOrderSink *Order
	benchmarkBookSink  *OrderBook
	benchmarkCloneSize int
	benchmarkTrades    int
)

func benchmarkBookAt() time.Time {
	return time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
}

func benchmarkRestingBook(b *testing.B, depth int, side Side) *OrderBook {
	b.Helper()

	book := NewOrderBook()
	at := benchmarkBookAt()

	for i := 0; i < depth; i++ {
		order, err := NewOrder(
			fmt.Sprintf("resting-%06d", i),
			"BTCUSD",
			side,
			Price(10_000+i%100),
			Quantity(1_000),
			at,
		)
		if err != nil {
			b.Fatalf("create resting order: %v", err)
		}
		if err := order.Open(at); err != nil {
			b.Fatalf("open resting order: %v", err)
		}
		if err := book.Add(order); err != nil {
			b.Fatalf("add resting order: %v", err)
		}
	}

	return book
}

// Measure the current sort-on-read behavior as resting depth increases.
func BenchmarkOrderBookTopOfBook(b *testing.B) {
	for _, depth := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			book := benchmarkRestingBook(b, depth, SideBuy)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				bids := book.Bids()
				if len(bids) != depth {
					b.Fatalf("expected %d bids, got %d", depth, len(bids))
				}
				benchmarkOrderSink = bids[0]
			}
		})
	}
}

// Measure direct best-price selection used by the matching engine. The
// sorted-view benchmark above remains as the pre-optimization comparison.
func BenchmarkOrderBookBestBid(b *testing.B) {
	for _, depth := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			book := benchmarkRestingBook(b, depth, SideBuy)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				entry, ok := book.bestBid()
				if !ok {
					b.Fatal("nonempty bid book has no best price")
				}
				benchmarkOrderSink = entry.order
			}
		})
	}
}

func BenchmarkOrderBookBestAsk(b *testing.B) {
	for _, depth := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			book := benchmarkRestingBook(b, depth, SideSell)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				entry, ok := book.bestAsk()
				if !ok {
					b.Fatal("nonempty ask book has no best price")
				}
				benchmarkOrderSink = entry.order
			}
		})
	}
}

// Clone cost matters because every service submission stages its symbol book.
func BenchmarkOrderBookClone(b *testing.B) {
	for _, depth := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			book := benchmarkRestingBook(b, depth, SideBuy)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				cloned, orders := book.Clone()
				benchmarkBookSink = cloned
				benchmarkCloneSize = len(orders)
			}
		})
	}
}

// Match a single aggressive BUY against many resting SELL orders. A cloned
// fixture is prepared outside the measured interval for each iteration.
func BenchmarkMatchingEngineCrossingBook(b *testing.B) {
	for _, depth := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("trades_%d", depth), func(b *testing.B) {
			book := benchmarkRestingBook(b, depth, SideSell)
			at := benchmarkBookAt().Add(time.Second)
			buy, err := NewOrder(
				"aggressive-buy",
				"BTCUSD",
				SideBuy,
				Price(20_000),
				Quantity(depth*1_000),
				at,
			)
			if err != nil {
				b.Fatalf("create aggressive order: %v", err)
			}
			if err := buy.Open(at); err != nil {
				b.Fatalf("open aggressive order: %v", err)
			}
			if err := book.Add(buy); err != nil {
				b.Fatalf("add aggressive order: %v", err)
			}

			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				working, _ := book.Clone()
				engine := NewMatchingEngine()
				b.StartTimer()

				trades, err := engine.Match(working, at)

				b.StopTimer()
				if err != nil {
					b.Fatalf("matching depth %d: %v", depth, err)
				}
				if len(trades) != depth {
					b.Fatalf("expected %d trades, got %d", depth, len(trades))
				}
				benchmarkTrades = len(trades)
			}
		})
	}
}
