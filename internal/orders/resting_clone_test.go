package orders

import (
	"fmt"
	"testing"
	"time"
)

func TestWouldCrossRespectsBestOppositePrice(t *testing.T) {
	book := NewOrderBook()
	for _, side := range []Side{SideBuy, SideSell} {
		if book.WouldCross(side, Price(100)) {
			t.Fatalf("empty book reports crossing for %s", side)
		}
	}

	at := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	add := func(id string, side Side, price Price) {
		t.Helper()
		o, err := NewOrder(id, "BTCUSD", side, price, Quantity(100), at)
		if err != nil {
			t.Fatal(err)
		}
		if err := o.Open(at); err != nil {
			t.Fatal(err)
		}
		if err := book.Add(o); err != nil {
			t.Fatal(err)
		}
	}
	add("bid-100", SideBuy, Price(100))
	add("bid-90", SideBuy, Price(90))
	add("ask-105", SideSell, Price(105))
	add("ask-110", SideSell, Price(110))

	for _, tt := range []struct {
		side  Side
		price Price
		want  bool
	}{
		{SideBuy, Price(104), false},
		{SideBuy, Price(105), true},
		{SideBuy, Price(106), true},
		{SideSell, Price(101), false},
		{SideSell, Price(100), true},
		{SideSell, Price(99), true},
	} {
		if got := book.WouldCross(tt.side, tt.price); got != tt.want {
			t.Errorf("side=%s price=%d got=%v want=%v", tt.side, tt.price, got, tt.want)
		}
	}
}

func TestCloneForRestingDetachesMapsAndPreservesFIFO(t *testing.T) {
	at := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	book := NewOrderBook()
	for i := 0; i < 3; i++ {
		order, err := NewOrder(fmt.Sprintf("sell-%d", i), "BTCUSD", SideSell,
			Price(100), Quantity(100), at)
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
	cloned := book.CloneForResting()

	if cloned.nextSequence != book.nextSequence {
		t.Errorf("next sequence changed: got %d, want %d", cloned.nextSequence, book.nextSequence)
	}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("sell-%d", i)
		origSeq, origOk := book.SequenceFor(id)
		cloneSeq, cloneOk := cloned.SequenceFor(id)
		if !origOk || !cloneOk || origSeq != cloneSeq {
			t.Errorf("missing or changed FIFO sequence for %s", id)
		}
		if cloned.asks[id].order != book.asks[id].order {
			t.Errorf("resting clone unexpectedly copied existing order %s", id)
		}
	}
	newOrder, err := NewOrder("sell-3", "BTCUSD", SideSell, Price(100), Quantity(100), at)
	if err != nil {
		t.Fatal(err)
	}
	if err := newOrder.Open(at); err != nil {
		t.Fatal(err)
	}
	if err := cloned.Add(newOrder); err != nil {
		t.Fatal(err)
	}
	if _, ok := book.SequenceFor("sell-3"); ok {
		t.Error("new order leaked into original book")
	}
	if _, ok := cloned.SequenceFor("sell-3"); !ok {
		t.Error("new order missing from the staged book")
	}
	if got := len(book.Asks()); got != 3 {
		t.Errorf("original book was modified: %d asks", got)
	}
	if got := len(cloned.Asks()); got != 4 {
		t.Errorf("staged book missing ask: %d", got)
	}
	if err := cloned.Remove("sell-0"); err != nil {
		t.Fatal(err)
	}
	if _, ok := book.SequenceFor("sell-0"); !ok {
		t.Error("deleting a staged map entry changed original map")
	}
}
