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

type blockingTradeCommitStore struct {
	entered chan struct{}
	release chan struct{}
	active atomic.Int32
	maxActive atomic.Int32
}

func (s *blockingTradeCommitStore) Apply(ctx context.Context, change StateChange) error {
	if len(change.Trades) == 0 {
		return ctx.Err()
	}
	active := s.active.Add(1)
	for {
		prev := s.maxActive.Load()
		if prev >= active || s.maxActive.CompareAndSwap(prev, active) {
			break
		}
	}
	defer s.active.Add(-1)
	if change.Trades[0].ID() == "trade-000001" {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestMatchedCommitReleasesRegistryWhilePersisting(t *testing.T) {
	store := &blockingTradeCommitStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	defer release()
	service := NewServiceWithStore(store)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := service.SubmitOrder(ctx, restingInput("sell-btc", "BTCUSD")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitOrder(ctx, restingInput("sell-eth", "ETHUSD")); err != nil {
		t.Fatal(err)
	}

	matchedDone := make(chan error, 1)
	go func() {
		_, err := service.SubmitOrder(ctx, SubmitOrderInput{
			ID: "buy-btc", Symbol: "BTCUSD", Side: orders.SideSell,
			Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
		})
		matchedDone <- err
	}()
	// The resting orders above are BUY. An aggressive SELL crosses BTC.
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("matching commit did not reach persistence")
	}

	// Registry reads, resting writes and cancellation on a separate symbol
	// must complete while the global trade write is still blocked.
	done := make(chan error, 1)
	go func() {
		old, err := service.GetOrder("sell-btc")
		if err != nil || old.Status != orders.OrderStatusOpen {
			done <- errors.New("matched book published before commit")
			return
		}
		if _, err := service.SubmitOrder(ctx, restingInput("rest-sol", "SOLUSD")); err != nil {
			done <- err
			return
		}
		_, err = service.CancelOrder(ctx, "sell-eth")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("independent operation failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("registry and unrelated writes blocked on trade commit")
	}

	// The ID of the uncommitted trade-producing order is globally reserved.
	_, err := service.SubmitOrder(ctx, restingInput("buy-btc", "DOGEUSD"))
	if !errors.Is(err, ErrDuplicateOrder) {
		t.Fatalf("reserved matching order ID should conflict: %v", err)
	}

	release()
	if err := <-matchedDone; err != nil {
		t.Fatalf("matching commit: %v", err)
	}
	if got := service.engine.TradeSequence(); got != 1 {
		t.Errorf("expected one confirmed trade ID, got %d", got)
	}
	if got, err := service.GetOrder("sell-eth"); err != nil ||
		got.Status != orders.OrderStatusCancelled {
		t.Errorf("cancellation lost: %+v err=%v", got, err)
	}
}

func TestIndependentMatchedCommitsKeepConsecutiveGlobalIDs(t *testing.T) {
	store := &blockingTradeCommitStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	defer release()
	service := NewServiceWithStore(store)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, symbol := range []string{"BTCUSD", "ETHUSD"} {
		if _, err := service.SubmitOrder(ctx, restingInput("rest-"+symbol, symbol)); err != nil {
			t.Fatal(err)
		}
	}

	ids := make(chan string, 2)
	errs := make(chan error, 2)
	submit := func(id, symbol string) {
		res, err := service.SubmitOrder(ctx, SubmitOrderInput{
			ID: id, Symbol: symbol, Side: orders.SideSell,
			Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
		})
		if err == nil && len(res.Trades) != 1 {
			err = errors.New("expected one execution")
		}
		if err == nil {
			ids <- res.Trades[0].ID()
		}
		errs <- err
	}
	go submit("cross-btc", "BTCUSD")
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first trade did not reach storage")
	}
	go submit("cross-eth", "ETHUSD")
	release()
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("matching order failed: %v", err)
		}
	}
	first, second := <-ids, <-ids
	if !(first == "trade-000001" && second == "trade-000002") &&
		!(second == "trade-000001" && first == "trade-000002") {
		t.Errorf("non-consecutive trades: %q %q", first, second)
	}
	if got := store.maxActive.Load(); got != 1 {
		t.Errorf("trade writes overlapped: max concurrent Apply calls = %d", got)
	}
}

type failFirstTradeStore struct {
	calls atomic.Int32
}

func (s *failFirstTradeStore) Apply(ctx context.Context, change StateChange) error {
	if len(change.Trades) > 0 && s.calls.Add(1) == 1 {
		return errors.New("transaction rejected")
	}
	return ctx.Err()
}

func TestFailedCoordinatedTradeReusesSequenceAndReleasesID(t *testing.T) {
	store := &failFirstTradeStore{}
	service := NewServiceWithStore(store)
	ctx := context.Background()
	if _, err := service.SubmitOrder(ctx, restingInput("rest-btc", "BTCUSD")); err != nil {
		t.Fatal(err)
	}

	tradingInput := SubmitOrderInput{
		ID: "cross-btc", Symbol: "BTCUSD", Side: orders.SideSell,
		Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
	}
	if _, err := service.SubmitOrder(ctx, tradingInput); err == nil {
		t.Fatal("first trade commit should fail")
	}
	if got := service.engine.TradeSequence(); got != 0 {
		t.Fatalf("failed trade consumed confirmed sequence %d", got)
	}
	if _, err := service.GetOrder("cross-btc"); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("failed trade order published: %v", err)
	}

	result, err := service.SubmitOrder(ctx, tradingInput)
	if err != nil {
		t.Fatalf("retry after failed transaction: %v", err)
	}
	if len(result.Trades) != 1 || result.Trades[0].ID() != "trade-000001" {
		t.Fatalf("retry did not reuse global sequence: %+v", result.Trades)
	}
}
