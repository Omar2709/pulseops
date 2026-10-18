package trading

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

type concurrentRestingStore struct {
	entered chan string
	release chan struct{}
}

func (store *concurrentRestingStore) Apply(ctx context.Context, change StateChange) error {
	if len(change.Orders) != 1 {
		return errors.New("expected one resting order")
	}
	select {
	case store.entered <- change.Orders[0].ID:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-store.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type heldRestingStore struct {
	entered chan struct{}
	release chan struct{}
	fail    error
	calls   atomic.Int32
}

func (store *heldRestingStore) Apply(ctx context.Context, change StateChange) error {
	if store.calls.Add(1) == 1 {
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

func restingInput(id, symbol string) SubmitOrderInput {
	return SubmitOrderInput{
		ID:       id,
		Symbol:   symbol,
		Side:     orders.SideBuy,
		Price:    orders.Price(10_000),
		Quantity: orders.Quantity(1_000),
	}
}

func awaitRestingID(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case id := <-ch:
		return id
	case <-time.After(5 * time.Second):
		t.Fatal("resting write did not enter the store")
		return ""
	}
}

func awaitRestingSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("resting write did not reach the store")
	}
}

func TestRestingOrdersFromDifferentSymbolsPersistConcurrently(t *testing.T) {
	store := &concurrentRestingStore{
		entered: make(chan string, 2),
		release: make(chan struct{}),
	}
	service := NewServiceWithStore(store)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(store.release) }) }
	defer release()

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, input := range []SubmitOrderInput{
		restingInput("btc", "BTCUSD"),
		restingInput("eth", "ETHUSD"),
	} {
		wg.Add(1)
		go func(input SubmitOrderInput) {
			defer wg.Done()
			_, err := service.SubmitOrder(ctx, input)
			results <- err
		}(input)
	}

	first := awaitRestingID(t, store.entered)
	second := awaitRestingID(t, store.entered)
	if first == second {
		t.Fatalf("expected distinct resting orders, received %s twice", first)
	}
	release()
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("persisting independent resting order: %v", err)
		}
	}
	for _, id := range []string{"btc", "eth"} {
		if _, err := service.GetOrder(id); err != nil {
			t.Errorf("missing committed order %s: %v", id, err)
		}
	}
	if got := service.engine.TradeSequence(); got != 0 {
		t.Errorf("resting writes advanced global trade sequence: %d", got)
	}
}

func TestConcurrentRestingOrderReservationRejectsCrossSymbolID(t *testing.T) {
	store := &heldRestingStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	service := NewServiceWithStore(store)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(store.release) }) }
	defer release()

	firstResult := make(chan error, 1)
	go func() {
		_, err := service.SubmitOrder(ctx, restingInput("shared", "BTCUSD"))
		firstResult <- err
	}()
	awaitRestingSignal(t, store.entered)

	secondResult := make(chan error, 1)
	go func() {
		_, err := service.SubmitOrder(ctx, restingInput("shared", "ETHUSD"))
		secondResult <- err
	}()

	select {
	case err := <-secondResult:
		if !errors.Is(err, ErrDuplicateOrder) {
			t.Errorf("expected global ID reservation conflict, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("duplicate ID submission waited for unrelated durable write")
	}

	release()
	if err := <-firstResult; err != nil {
		t.Fatalf("committing reserved order: %v", err)
	}
	if got := store.calls.Load(); got != 1 {
		t.Errorf("duplicate order reached storage: %d calls", got)
	}
	if order, err := service.GetOrder("shared"); err != nil || order.Symbol != "BTCUSD" {
		t.Errorf("wrong committed order: %+v, err=%v", order, err)
	}
}

func TestFailedConcurrentRestingWriteReleasesOrderID(t *testing.T) {
	storageError := errors.New("durable insert failed")
	store := &heldRestingStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		fail:    storageError,
	}
	service := NewServiceWithStore(store)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(store.release) }) }
	defer release()

	firstResult := make(chan error, 1)
	go func() {
		_, err := service.SubmitOrder(ctx, restingInput("retry-id", "BTCUSD"))
		firstResult <- err
	}()
	awaitRestingSignal(t, store.entered)
	release()

	if err := <-firstResult; !errors.Is(err, storageError) {
		t.Fatalf("expected storage error, got %v", err)
	}
	if _, err := service.GetOrder("retry-id"); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("failed resting write became visible: %v", err)
	}
	result, err := service.SubmitOrder(ctx, restingInput("retry-id", "ETHUSD"))
	if err != nil {
		t.Fatalf("retry after failed write: %v", err)
	}
	if result.Order.Symbol != "ETHUSD" || result.Replayed {
		t.Errorf("unexpected retried order: %+v", result)
	}
	if store.calls.Load() != 2 {
		t.Errorf("expected retry to reach store")
	}
}

type selectiveHeldStore struct {
	entered chan struct{}
	release chan struct{}
}

func (store *selectiveHeldStore) Apply(ctx context.Context, change StateChange) error {
	if len(change.Orders) == 1 && change.Orders[0].ID == "blocked-resting" {
		close(store.entered)
		select {
		case <-store.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestDelayedRestingCommitNeverResetsConcurrentTradeSequence(t *testing.T) {
	store := &selectiveHeldStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	service := NewServiceWithStore(store)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(store.release) }) }
	defer release()

	restingDone := make(chan error, 1)
	go func() {
		_, err := service.SubmitOrder(ctx, restingInput("blocked-resting", "BTCUSD"))
		restingDone <- err
	}()
	awaitRestingSignal(t, store.entered)

	tradeDone := make(chan error, 1)
	go func() {
		sell := SubmitOrderInput{
			ID: "sell-eth", Symbol: "ETHUSD", Side: orders.SideSell,
			Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
		}
		if _, err := service.SubmitOrder(ctx, sell); err != nil {
			tradeDone <- err
			return
		}
		buy := restingInput("buy-eth", "ETHUSD")
		result, err := service.SubmitOrder(ctx, buy)
		if err == nil && (len(result.Trades) != 1 || result.Trades[0].ID() != "trade-000001") {
			err = errors.New("matching did not produce first global trade")
		}
		tradeDone <- err
	}()
	select {
	case err := <-tradeDone:
		if err != nil {
			t.Fatalf("matching while resting write is blocked: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("matched order blocked behind unrelated resting persistence")
	}
	release()
	if err := <-restingDone; err != nil {
		t.Fatalf("delayed resting write: %v", err)
	}
	if got := service.engine.TradeSequence(); got != 1 {
		t.Errorf("delayed resting publication reset trade sequence to %d", got)
	}
	if _, err := service.GetOrder("blocked-resting"); err != nil {
		t.Errorf("resting order was not published: %v", err)
	}
}
