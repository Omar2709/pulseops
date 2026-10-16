package orders

import (
	"fmt"
	"testing"
	"time"
)

func TestBestEntriesAgreeWithSortedViews(t *testing.T) {
	book := NewOrderBook()
	verify := func(label string) {
		t.Helper()

		bid, ok := book.bestBid()
		sortedBids := book.Bids()
		if ok != (len(sortedBids) != 0) {
			t.Fatalf("%s: bestBid existence mismatch", label)
		}
		if ok {
			expected := sortedBids[0].ID()
			if bid.order.ID() != expected {
				t.Errorf("%s: best bid %q, sorted first %q", label, bid.order.ID(), expected)
			}
			if bid.sequence != book.bids[expected].sequence {
				t.Errorf("%s: best bid sequence changed", label)
			}
		}

		ask, ok := book.bestAsk()
		sortedAsks := book.Asks()
		if ok != (len(sortedAsks) != 0) {
			t.Fatalf("%s: bestAsk existence mismatch", label)
		}
		if ok {
			expected := sortedAsks[0].ID()
			if ask.order.ID() != expected {
				t.Errorf("%s: best ask %q, sorted first %q", label, ask.order.ID(), expected)
			}
			if ask.sequence != book.asks[expected].sequence {
				t.Errorf("%s: best ask sequence changed", label)
			}
		}
	}

	verify("empty")
	at := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

	// Intentionally interleave sides/prices, including many tied prices,
	// so the comparison detects an accidental loss of price-time FIFO.
	for i := 0; i < 240; i++ {
		side := SideBuy
		if i%3 == 0 {
			side = SideSell
		}
		price := Price(10_000 + (i*37)%19)
		order, err := NewOrder(
			fmt.Sprintf("order-%03d", i),
			"BTCUSD",
			side,
			price,
			Quantity(100),
			at,
		)
		if err != nil {
			t.Fatalf("create order %d: %v", i, err)
		}
		if err := order.Open(at); err != nil {
			t.Fatalf("open order %d: %v", i, err)
		}
		if err := book.Add(order); err != nil {
			t.Fatalf("add order %d: %v", i, err)
		}
	}
	verify("populated")

	cloned, _ := book.Clone()
	book = cloned
	verify("cloned")

	for i := 0; i < 240; i++ {
		if i%4 != 0 {
			continue
		}
		id := fmt.Sprintf("order-%03d", i)
		if err := book.Remove(id); err != nil {
			t.Fatalf("remove order %s: %v", id, err)
		}
	}
	verify("after removals")
}

func TestBestEntriesPreserveRestoredFIFO(t *testing.T) {
	book := NewOrderBook()
	at := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

	add := func(id string, side Side, price Price, sequence uint64) {
		t.Helper()
		order, err := NewOrder(id, "BTCUSD", side, price, Quantity(100), at)
		if err != nil {
			t.Fatalf("create order %s: %v", id, err)
		}
		if err := order.Open(at); err != nil {
			t.Fatalf("open order %s: %v", id, err)
		}
		if err := book.Restore(order, sequence); err != nil {
			t.Fatalf("restore %s: %v", id, err)
		}
	}

	// Recovery can insert orders out of FIFO order; the sequence, not
	// iteration order or creation timestamp, determines tie priority.
	add("bid-later", SideBuy, Price(10_100), 20)
	add("bid-earlier", SideBuy, Price(10_100), 4)
	add("bid-cheaper", SideBuy, Price(10_000), 1)
	add("ask-later", SideSell, Price(9_900), 21)
	add("ask-earlier", SideSell, Price(9_900), 5)
	add("ask-higher", SideSell, Price(10_000), 0)

	bid, ok := book.bestBid()
	if !ok || bid.order.ID() != "bid-earlier" {
		t.Errorf("expected earliest bid at best price, got %+v", bid)
	}
	ask, ok := book.bestAsk()
	if !ok || ask.order.ID() != "ask-earlier" {
		t.Errorf("expected earliest ask at best price, got %+v", ask)
	}

	if err := book.Remove("bid-earlier"); err != nil {
		t.Fatalf("remove bid: %v", err)
	}
	if err := book.Remove("ask-earlier"); err != nil {
		t.Fatalf("remove ask: %v", err)
	}

	bid, ok = book.bestBid()
	if !ok || bid.order.ID() != "bid-later" {
		t.Errorf("expected next bid FIFO priority, got %+v", bid)
	}
	ask, ok = book.bestAsk()
	if !ok || ask.order.ID() != "ask-later" {
		t.Errorf("expected next ask FIFO priority, got %+v", ask)
	}
}

func TestBestEntriesMatchReferenceAfterMatchingAndCancellation(t *testing.T) {
	at := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	book := NewOrderBook()
	for i, side := range []Side{SideSell, SideSell, SideBuy} {
		id := fmt.Sprintf("order-%d", i)
		price := Price(100)
		if side == SideBuy {
			price = Price(99)
		}
		order, err := NewOrder(id, "BTCUSD", side, price, Quantity(100), at)
		if err != nil {
			t.Fatal(err)
		}
		if err := order.Open(at); err != nil {
			t.Fatal(err)
		}
		if err := book.Add(order); err != nil {
			t.Fatal(err)
		}
	}

	// Cancelling the first resting ask must expose the second ask.
	if err := book.Cancel("order-0", at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	ask, ok := book.bestAsk()
	if !ok || ask.order.ID() != "order-1" {
		t.Errorf("cancelled ask still has priority: %+v", ask)
	}
}
