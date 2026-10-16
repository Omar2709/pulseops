package trading

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

func TestSubmitOrderReplayDoesNotRematchOrPersist(t *testing.T) {
	service := newService(func() time.Time { return testTime() })

	sell := SubmitOrderInput{
		ID:       "sell-001",
		Symbol:   "BTCUSD",
		Side:     orders.SideSell,
		Price:    orders.Price(6_000_000),
		Quantity: orders.Quantity(1_000_000),
	}
	if _, err := service.SubmitOrder(context.Background(), sell); err != nil {
		t.Fatalf("submitting resting order: %v", err)
	}

	buy := SubmitOrderInput{
		ID:       "buy-001",
		Symbol:   "BTCUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(6_100_000),
		Quantity: orders.Quantity(1_000_000),
	}
	first, err := service.SubmitOrder(context.Background(), buy)
	if err != nil {
		t.Fatalf("submitting matching order: %v", err)
	}
	if first.Replayed || len(first.Trades) != 1 {
		t.Fatalf("expected one original execution, got %+v", first)
	}

	// The retry must not touch storage even if storage is unavailable.
	store := &recordingStateStore{err: errors.New("store unavailable")}
	service.store = store
	buy.Symbol = " btcusd "

	replay, err := service.SubmitOrder(context.Background(), buy)
	if err != nil {
		t.Fatalf("replaying order: %v", err)
	}
	if !replay.Replayed || len(replay.Trades) != 0 {
		t.Errorf("replay re-executed matching: %+v", replay)
	}
	if replay.Order != first.Order {
		t.Errorf("replay did not return the committed order snapshot")
	}
	if store.ctx != nil {
		t.Error("replay unexpectedly called persistence")
	}
	if got := service.engine.TradeSequence(); got != 1 {
		t.Errorf("expected trade sequence 1, got %d", got)
	}

	buy.Price = orders.Price(6_200_000)
	if _, err := service.SubmitOrder(context.Background(), buy); !errors.Is(err, ErrDuplicateOrder) {
		t.Errorf("expected conflict on changed immutable attributes, got %v", err)
	}
}

func TestSubmitOrderReplayReturnsCurrentCancelledState(t *testing.T) {
	service := newService(func() time.Time { return testTime() })
	input := SubmitOrderInput{
		ID:       "buy-001",
		Symbol:   "BTCUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(6_000_000),
		Quantity: orders.Quantity(1_000_000),
	}
	if _, err := service.SubmitOrder(context.Background(), input); err != nil {
		t.Fatalf("submitting order: %v", err)
	}
	if _, err := service.CancelOrder(context.Background(), input.ID); err != nil {
		t.Fatalf("cancelling order: %v", err)
	}
	replay, err := service.SubmitOrder(context.Background(), input)
	if err != nil {
		t.Fatalf("replaying cancelled order: %v", err)
	}
	if !replay.Replayed || replay.Order.Status != orders.OrderStatusCancelled {
		t.Errorf("expected cancelled order snapshot on replay: %+v", replay)
	}
	if got := len(service.books["BTCUSD"].Bids()); got != 0 {
		t.Errorf("replay resurrected %d resting bids", got)
	}
}

func TestSubmitOrderConcurrentIdenticalRequests(t *testing.T) {
	service := NewService()
	input := SubmitOrderInput{
		ID:       "same-id",
		Symbol:   "ETHUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(200_000),
		Quantity: orders.Quantity(1_000_000),
	}
	const requests = 20

	type outcome struct {
		result SubmitOrderResult
		err    error
	}
	results := make(chan outcome, requests)
	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := service.SubmitOrder(context.Background(), input)
			results <- outcome{result: result, err: err}
		}()
	}
	wg.Wait()
	close(results)

	var created, replayed int
	for item := range results {
		if item.err != nil {
			t.Fatalf("concurrent replay failed: %v", item.err)
		}
		if item.result.Replayed {
			replayed++
		} else {
			created++
		}
	}
	if created != 1 || replayed != requests-1 {
		t.Errorf("expected one creation and %d replays, got %d and %d",
			requests-1, created, replayed)
	}
	if len(service.orders) != 1 || len(service.books["ETHUSD"].Bids()) != 1 {
		t.Error("concurrent retries created duplicate orders")
	}
}
