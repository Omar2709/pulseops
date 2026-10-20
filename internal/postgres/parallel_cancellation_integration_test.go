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

// holdCommittedCancellationStore pauses after a cancellation is durable,
// but before the trading service publishes the new state in memory.
type holdCommittedCancellationStore struct {
	inner   *Store
	entered chan struct{}
	release chan struct{}
}

func (store *holdCommittedCancellationStore) Apply(ctx context.Context, change trading.StateChange) error {
	if err := store.inner.Apply(ctx, change); err != nil {
		return err
	}
	if len(change.Orders) == 1 && change.Orders[0].Status == orders.OrderStatusCancelled {
		close(store.entered)
		select {
		case <-store.release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestPostgresCancellationAndIndependentMatchPreserveRecovery(t *testing.T) {
	pool := testPostgresPool(t)
	db, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	store := &holdCommittedCancellationStore{
		inner: db, entered: make(chan struct{}), release: make(chan struct{}),
	}
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	service := trading.NewServiceWithStore(store)

	if _, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
		ID: "cancel-btc", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
	}); err != nil {
		t.Fatal(err)
	}

	cancelDone := make(chan error, 1)
	go func() {
		_, err := service.CancelOrder(ctx, "cancel-btc")
		cancelDone <- err
	}()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not commit to PostgreSQL")
	}

	// The previous order state remains visible to registry readers until
	// the cancellation publishes, but another symbol must be able to trade.
	before, err := service.GetOrder("cancel-btc")
	if err != nil || before.Status != orders.OrderStatusOpen {
		t.Fatalf("registry published cancellation early: %+v err=%v", before, err)
	}

	tradeDone := make(chan error, 1)
	go func() {
		if _, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
			ID: "sell-eth", Symbol: "ETHUSD", Side: orders.SideSell,
			Price: orders.Price(20_000), Quantity: orders.Quantity(1_000),
		}); err != nil {
			tradeDone <- err
			return
		}
		result, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
			ID: "buy-eth", Symbol: "ETHUSD", Side: orders.SideBuy,
			Price: orders.Price(20_000), Quantity: orders.Quantity(1_000),
		})
		if err == nil && (len(result.Trades) != 1 || result.Trades[0].ID() != "trade-000001") {
			err = fmt.Errorf("unexpected trade during concurrent cancellation: %+v", result.Trades)
		}
		tradeDone <- err
	}()
	select {
	case err := <-tradeDone:
		if err != nil {
			t.Fatalf("ETH match did not commit independently: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ETH matching blocked behind committed cancellation")
	}

	release()
	if err := <-cancelDone; err != nil {
		t.Fatalf("publish cancellation: %v", err)
	}

	recoveredState, err := db.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredState.TradeSequence != 1 || len(recoveredState.Orders) != 3 {
		t.Fatalf("unexpected durable state: sequence=%d orders=%d",
			recoveredState.TradeSequence, len(recoveredState.Orders))
	}
	recovered, err := trading.NewRecoveredService(db, recoveredState)
	if err != nil {
		t.Fatal(err)
	}
	btc, err := recovered.GetOrder("cancel-btc")
	if err != nil || btc.Status != orders.OrderStatusCancelled {
		t.Errorf("BTC cancellation lost after restart: %+v err=%v", btc, err)
	}
	for _, id := range []string{"sell-eth", "buy-eth"} {
		o, err := recovered.GetOrder(id)
		if err != nil || o.Status != orders.OrderStatusFilled {
			t.Errorf("trade order %s not recovered: %+v err=%v", id, o, err)
		}
	}
	if _, err := recovered.SubmitOrder(ctx, trading.SubmitOrderInput{
		ID: "sell-next", Symbol: "ETHUSD", Side: orders.SideSell,
		Price: orders.Price(20_000), Quantity: orders.Quantity(1_000),
	}); err != nil {
		t.Fatal(err)
	}
	next, err := recovered.SubmitOrder(ctx, trading.SubmitOrderInput{
		ID: "buy-next", Symbol: "ETHUSD", Side: orders.SideBuy,
		Price: orders.Price(20_000), Quantity: orders.Quantity(1_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Trades) != 1 || next.Trades[0].ID() != "trade-000002" {
		t.Errorf("sequence regression after PostgreSQL recovery: %+v", next.Trades)
	}
}
