package orders

import (
	"errors"
	"testing"
	"time"
)

func TestOrderBookRestorePreservesSequenceAndContinuesAfterMaximum(
	t *testing.T,
) {
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

	makeOrder := func(id string) *Order {
		t.Helper()

		order, err := NewOrder(
			id,
			"BTCUSD",
			SideSell,
			Price(6_000_000),
			Quantity(1_000_000),
			at,
		)
		if err != nil {
			t.Fatalf("creating %s: %v", id, err)
		}

		if err := order.Open(at); err != nil {
			t.Fatalf("opening %s: %v", id, err)
		}

		return order
	}

	book := NewOrderBook()

	if err := book.Restore(
		makeOrder("sell-later"),
		9,
	); err != nil {
		t.Fatalf("restoring later order: %v", err)
	}

	if err := book.Restore(
		makeOrder("sell-priority"),
		3,
	); err != nil {
		t.Fatalf("restoring priority order: %v", err)
	}

	asks := book.Asks()
	if len(asks) != 2 ||
		asks[0].ID() != "sell-priority" ||
		asks[1].ID() != "sell-later" {
		t.Fatalf("recovery changed FIFO priority")
	}

	if err := book.Add(makeOrder("sell-new")); err != nil {
		t.Fatalf("adding new order: %v", err)
	}

	sequences := book.Sequences()

	if got := sequences["sell-new"]; got != 10 {
		t.Errorf(
			"expected new sequence 10, got %d",
			got,
		)
	}

	if err := book.Restore(
		makeOrder("sell-duplicate-sequence"),
		3,
	); !errors.Is(err, ErrOrderBookDuplicateSequence) {
		t.Fatalf(
			"expected duplicate sequence error, got %v",
			err,
		)
	}
}
