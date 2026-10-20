package trading

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

type blockedCancellationStore struct {
	entered chan struct{}
	release chan struct{}
	fail    error
}

func (store *blockedCancellationStore) Apply(ctx context.Context, change StateChange) error {
	if len(change.Orders) == 1 && change.Orders[0].Status == orders.OrderStatusCancelled {
		close(store.entered)
		select {
		case <-store.release:
			return store.fail
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func awaitCancellation(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation never entered persistence")
	}
}

func TestCancellationPersistenceDoesNotBlockDifferentSymbolMatching(t *testing.T) {
	store := &blockedCancellationStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	service := NewServiceWithStore(store)
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_, err := service.SubmitOrder(ctx, SubmitOrderInput{
		ID: "buy-btc", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(100), Quantity: orders.Quantity(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := service.CancelOrder(ctx, "buy-btc")
		done <- err
	}()
	awaitCancellation(t, store.entered)

	// The in-memory state remains unchanged until persistence completes.
	before, err := service.GetOrder("buy-btc")
	if err != nil || before.Status != orders.OrderStatusOpen {
		t.Fatalf("cancellation published too early: %+v err=%v", before, err)
	}

	matched := make(chan error, 1)
	go func() {
		if _, err := service.SubmitOrder(ctx, SubmitOrderInput{
			ID: "sell-eth", Symbol: "ETHUSD", Side: orders.SideSell,
			Price: orders.Price(200), Quantity: orders.Quantity(100),
		}); err != nil {
			matched <- err
			return
		}
		result, err := service.SubmitOrder(ctx, SubmitOrderInput{
			ID: "buy-eth", Symbol: "ETHUSD", Side: orders.SideBuy,
			Price: orders.Price(200), Quantity: orders.Quantity(100),
		})
		if err == nil && (len(result.Trades) != 1 || result.Trades[0].ID() != "trade-000001") {
			err = errors.New("matching missed or duplicated the global trade")
		}
		matched <- err
	}()

	select {
	case err := <-matched:
		if err != nil {
			t.Fatalf("other symbol matching failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("other symbol matching blocked behind cancellation persistence")
	}

	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	after, err := service.GetOrder("buy-btc")
	if err != nil || after.Status != orders.OrderStatusCancelled {
		t.Fatalf("cancellation was not published: %+v err=%v", after, err)
	}
	if got := service.engine.TradeSequence(); got != 1 {
		t.Errorf("cancellation overwrote global trade sequence: %d", got)
	}
}

func TestCancellationPersistenceSerializesSameSymbol(t *testing.T) {
	store := &blockedCancellationStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	service := NewServiceWithStore(store)
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_, err := service.SubmitOrder(ctx, SubmitOrderInput{
		ID: "sell-1", Symbol: "BTCUSD", Side: orders.SideSell,
		Price: orders.Price(100), Quantity: orders.Quantity(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := service.CancelOrder(ctx, "sell-1")
		done <- err
	}()
	awaitCancellation(t, store.entered)

	started := make(chan struct{})
	next := make(chan SubmitOrderResult, 1)
	errs := make(chan error, 1)
	go func() {
		close(started)
		result, err := service.SubmitOrder(ctx, SubmitOrderInput{
			ID: "buy-2", Symbol: "BTCUSD", Side: orders.SideBuy,
			Price: orders.Price(101), Quantity: orders.Quantity(100),
		})
		next <- result
		errs <- err
	}()
	<-started
	// Ensure no same-symbol publication occurs while cancellation's symbol
	// gate is held, even though global readers can still read old state.
	select {
	case <-next:
		t.Fatal("same-symbol submission completed before cancellation was published")
	default:
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	result := <-next
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 0 {
		t.Errorf("BUY matched a cancelled SELL: %+v", result.Trades)
	}
	sell, err := service.GetOrder("sell-1")
	if err != nil || sell.Status != orders.OrderStatusCancelled {
		t.Errorf("sell not cancelled: %+v err=%v", sell, err)
	}
}

func TestCancellationPersistenceFailureLeavesBookAndSequenceUntouched(t *testing.T) {
	store := &blockedCancellationStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		fail:    errors.New("storage unavailable"),
	}
	service := NewServiceWithStore(store)
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, err := service.SubmitOrder(ctx, SubmitOrderInput{
		ID: "sell-1", Symbol: "BTCUSD", Side: orders.SideSell,
		Price: orders.Price(100), Quantity: orders.Quantity(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	before := service.books["BTCUSD"]
	done := make(chan error, 1)
	go func() {
		_, err := service.CancelOrder(ctx, "sell-1")
		done <- err
	}()
	awaitCancellation(t, store.entered)
	release()
	if err := <-done; !errors.Is(err, store.fail) {
		t.Fatalf("expected storage failure, got %v", err)
	}
	if service.books["BTCUSD"] != before {
		t.Error("failed cancellation published a different order book")
	}
	state, err := service.GetOrder("sell-1")
	if err != nil || state.Status != orders.OrderStatusOpen {
		t.Errorf("failed cancellation changed resting order: %+v err=%v", state, err)
	}
}
