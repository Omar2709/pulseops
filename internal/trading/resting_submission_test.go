package trading

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

func TestNonCrossingSubmissionPersistsOnlyIncomingOrder(t *testing.T) {
	service := newService(func() time.Time { return testTime() })
	first := SubmitOrderInput{
		ID: "sell-1", Symbol: "BTCUSD", Side: orders.SideSell,
		Price: orders.Price(6_000_000), Quantity: orders.Quantity(1_000_000),
	}
	if _, err := service.SubmitOrder(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	oldOrder := service.orders[first.ID]
	oldBook := service.books[first.Symbol]

	recorder := &recordingStateStore{}
	service.store = recorder
	second := first
	second.ID = "sell-2"
	result, err := service.SubmitOrder(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || len(result.Trades) != 0 || result.Order.Status != orders.OrderStatusOpen {
		t.Errorf("unexpected staged resting submission: %+v", result)
	}

	if len(recorder.change.Orders) != 1 || recorder.change.Orders[0].ID != "sell-2" {
		t.Errorf("expected only new order persisted, got %+v", recorder.change.Orders)
	}
	if got := len(recorder.change.BookSequences); got != 1 {
		t.Fatalf("expected one changed FIFO sequence, got %d", got)
	}
	if seq, ok := recorder.change.BookSequences["sell-2"]; !ok || seq != 1 {
		t.Errorf("expected second sequence 1, got seq=%d ok=%v", seq, ok)
	}
	if got := recorder.change.TradeSequence; got != 0 {
		t.Errorf("unexpected global trade sequence %d", got)
	}
	if service.orders[first.ID] != oldOrder {
		t.Error("unchanged order entity was replaced")
	}
	if oldBook == service.books[first.Symbol] {
		t.Error("new order book was not committed as a new map snapshot")
	}
	if got := len(oldBook.Asks()); got != 1 {
		t.Errorf("original book was changed in place: %d asks", got)
	}

	// A subsequent crossing BUY must execute the first resting SELL
	// before the second, preserving exact FIFO sequence.
	service.store = noopStateStore{}
	buy, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID: "buy-1", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(6_000_000), Quantity: orders.Quantity(1_000_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(buy.Trades) != 1 || buy.Trades[0].SellOrderID() != "sell-1" {
		t.Errorf("resting FIFO changed after fast path: %+v", buy.Trades)
	}
}

func TestFailedNonCrossingSubmissionDoesNotPublish(t *testing.T) {
	service := newService(func() time.Time { return testTime() })
	if _, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID: "bid-existing", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(5_000_000), Quantity: orders.Quantity(1_000_000),
	}); err != nil {
		t.Fatal(err)
	}
	oldBook := service.books["BTCUSD"]
	oldOrder := service.orders["bid-existing"]
	expected := errors.New("persist unavailable")
	service.store = &recordingStateStore{err: expected}

	_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID: "bid-failed", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(5_000_000), Quantity: orders.Quantity(1_000_000),
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected persistence error, got %v", err)
	}
	if _, err := service.GetOrder("bid-failed"); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("failed order published: %v", err)
	}
	if service.books["BTCUSD"] != oldBook || service.orders["bid-existing"] != oldOrder {
		t.Error("failed operation replaced the book or resting entity")
	}
	if len(oldBook.Bids()) != 1 {
		t.Error("failed order changed the original book")
	}

	// Failed insert must not consume FIFO priority.
	service.store = &recordingStateStore{}
	if _, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID: "bid-valid", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(5_000_000), Quantity: orders.Quantity(1_000_000),
	}); err != nil {
		t.Fatal(err)
	}
	if got := len(service.books["BTCUSD"].Bids()); got != 2 {
		t.Fatalf("expected 2 bids after retry, got %d", got)
	}
	if seq, ok := service.books["BTCUSD"].SequenceFor("bid-valid"); !ok || seq != 1 {
		t.Errorf("failed insert consumed FIFO sequence: seq=%d ok=%v", seq, ok)
	}
}
