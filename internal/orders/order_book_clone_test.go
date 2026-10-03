package orders

import (
	"testing"
	"time"
)

func TestOrderBookClonePreservesPriorityAndIsolation(
	t *testing.T,
) {
	at := time.Date(
		2026, time.September, 26,
		14, 0, 0, 0,
		time.UTC,
	)

	original := NewOrderBook()

	for _, id := range []string{"buy-001", "buy-002"} {
		order, err := NewOrder(
			id,
			"BTCUSD",
			SideBuy,
			Price(6_000_000),
			Quantity(5_000_000),
			at,
		)
		if err != nil {
			t.Fatalf("creating %s: %v", id, err)
		}

		if err := order.Open(at); err != nil {
			t.Fatalf("opening %s: %v", id, err)
		}

		if err := original.Add(order); err != nil {
			t.Fatalf("adding %s: %v", id, err)
		}
	}

	cloned, clonedOrders := original.Clone()

	// Cloning must preserve FIFO priority.
	bids := cloned.Bids()

	if len(bids) != 2 {
		t.Fatalf("expected 2 cloned bids, got %d", len(bids))
	}

	if bids[0].ID() != "buy-001" ||
		bids[1].ID() != "buy-002" {
		t.Error("clone changed FIFO priority")
	}

	if cloned.nextSequence != original.nextSequence {
		t.Error("clone changed the insertion sequence")
	}

	// Copies must not share mutable Order entities.
	if clonedOrders["buy-001"] ==
		original.bids["buy-001"].order {
		t.Fatal("clone shares an order with the original")
	}

	if err := clonedOrders["buy-001"].ApplyFill(
		Quantity(2_000_000),
		at.Add(time.Second),
	); err != nil {
		t.Fatalf("filling cloned order: %v", err)
	}

	if original.bids["buy-001"].order.FilledQuantity() != 0 {
		t.Error("modifying the clone changed the original order")
	}

	// Removing from the clone must not modify the original book.
	if err := cloned.Remove("buy-001"); err != nil {
		t.Fatalf("removing cloned order: %v", err)
	}

	if len(original.Bids()) != 2 {
		t.Error("modifying the clone changed the original book")
	}
}

// TestOrderBookClonePreservesCrossSideSequence checks that cloning does
// not reset insertion priority when BUY and SELL orders were interleaved.
func TestOrderBookClonePreservesCrossSideSequence(t *testing.T) {
	at := time.Date(2026, time.September, 26, 14, 0, 0, 0, time.UTC)
	original := NewOrderBook()

	inputs := []struct {
		id    string
		side  Side
		price Price
	}{
		{"sell-first", SideSell, Price(6_000_000)},
		{"buy-second", SideBuy, Price(5_900_000)},
		{"sell-third", SideSell, Price(6_000_000)},
	}

	for _, input := range inputs {
		order, err := NewOrder(
			input.id, "BTCUSD", input.side,
			input.price, Quantity(1_000_000), at,
		)
		if err != nil {
			t.Fatalf("creating %s: %v", input.id, err)
		}
		if err := order.Open(at); err != nil {
			t.Fatalf("opening %s: %v", input.id, err)
		}
		if err := original.Add(order); err != nil {
			t.Fatalf("adding %s: %v", input.id, err)
		}
	}

	cloned, _ := original.Clone()

	if cloned.nextSequence != original.nextSequence {
		t.Fatal("clone changed nextSequence")
	}

	for id, entry := range original.bids {
		if copied, exists := cloned.bids[id]; !exists ||
			copied.sequence != entry.sequence {
			t.Errorf("BUY sequence changed for %s", id)
		}
	}
	for id, entry := range original.asks {
		if copied, exists := cloned.asks[id]; !exists ||
			copied.sequence != entry.sequence {
			t.Errorf("SELL sequence changed for %s", id)
		}
	}

	asks := cloned.Asks()
	if len(asks) != 2 || asks[0].ID() != "sell-first" ||
		asks[1].ID() != "sell-third" {
		t.Fatalf("clone changed SELL FIFO priority")
	}
}
