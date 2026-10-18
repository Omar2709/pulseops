package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
)

// delayingCommittedStore creates a deterministic commit-publication gap.
// The targeted resting order is already durable while its SubmitOrder is
// blocked waiting for the caller's permission to publish in memory.
type delayingCommittedStore struct {
	inner   *Store
	entered chan struct{}
	release chan struct{}
}

func (store *delayingCommittedStore) Apply(
	ctx context.Context,
	change trading.StateChange,
) error {
	if err := store.inner.Apply(ctx, change); err != nil {
		return err
	}
	if len(change.Orders) == 1 && change.Orders[0].ID == "resting-delayed" {
		close(store.entered)
		select {
		case <-store.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestPostgresConcurrentRestingAndMatchingRecovery(t *testing.T) {
	pool := testPostgresPool(t)
	inner, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	store := &delayingCommittedStore{
		inner:   inner,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	service := trading.NewServiceWithStore(store)

	restingDone := make(chan error, 1)
	go func() {
		_, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
			ID:       "resting-delayed",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(10_000),
			Quantity: orders.Quantity(1_000),
		})
		restingDone <- err
	}()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("resting submission never reached committed database state")
	}

	matchingDone := make(chan error, 1)
	go func() {
		sell := trading.SubmitOrderInput{
			ID:       "sell-eth",
			Symbol:   "ETHUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(20_000),
			Quantity: orders.Quantity(1_000),
		}
		if _, err := service.SubmitOrder(ctx, sell); err != nil {
			matchingDone <- fmt.Errorf("submit ETH sell: %w", err)
			return
		}
		buy := trading.SubmitOrderInput{
			ID:       "buy-eth",
			Symbol:   "ETHUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(20_000),
			Quantity: orders.Quantity(1_000),
		}
		result, err := service.SubmitOrder(ctx, buy)
		if err == nil && (len(result.Trades) != 1 ||
			result.Trades[0].ID() != "trade-000001") {
			err = fmt.Errorf("unexpected matching result: %+v", result.Trades)
		}
		matchingDone <- err
	}()

	// The ETH trade must persist even though the BTC insert has
	// committed but remains unpublished in the application service.
	select {
	case err := <-matchingDone:
		if err != nil {
			t.Fatalf("match on independent symbol: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ETH trade blocked behind unpublished BTC resting write")
	}
	release()
	if err := <-restingDone; err != nil {
		t.Fatalf("publish committed resting order: %v", err)
	}

	state, err := inner.Load(ctx)
	if err != nil {
		t.Fatalf("recover committed state: %v", err)
	}
	if state.TradeSequence != 1 || len(state.Orders) != 3 {
		t.Fatalf("inconsistent recovered state: sequence=%d orders=%d",
			state.TradeSequence, len(state.Orders))
	}

	recovered, err := trading.NewRecoveredService(inner, state)
	if err != nil {
		t.Fatalf("rebuild service: %v", err)
	}
	resting, err := recovered.GetOrder("resting-delayed")
	if err != nil || resting.Status != orders.OrderStatusOpen {
		t.Errorf("BTC resting order not recovered: %+v, err=%v", resting, err)
	}
	for _, id := range []string{"sell-eth", "buy-eth"} {
		snapshot, err := recovered.GetOrder(id)
		if err != nil || snapshot.Status != orders.OrderStatusFilled {
			t.Errorf("matched %s not recovered: %+v, err=%v", id, snapshot, err)
		}
	}

	// Recovery must continue the global sequence, not reuse an already
	// committed trade ID after unrelated resting commits complete later.
	if _, err := recovered.SubmitOrder(ctx, trading.SubmitOrderInput{
		ID:       "sell-next",
		Symbol:   "ETHUSD",
		Side:     orders.SideSell,
		Price:    orders.Price(20_000),
		Quantity: orders.Quantity(1_000),
	}); err != nil {
		t.Fatal(err)
	}
	next, err := recovered.SubmitOrder(ctx, trading.SubmitOrderInput{
		ID:       "buy-next",
		Symbol:   "ETHUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(20_000),
		Quantity: orders.Quantity(1_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Trades) != 1 || next.Trades[0].ID() != "trade-000002" {
		t.Errorf("trade sequence lost after concurrent recovery: %+v", next.Trades)
	}
}
