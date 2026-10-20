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

// pausedTradeStore deliberately keeps the application waiting after a trade
// transaction has committed in PostgreSQL but before it can be published.
// This models a slow acknowledgement and tests the commit coordinator.
type pausedTradeStore struct {
	inner   *Store
	entered chan struct{}
	release chan struct{}
}

func (s *pausedTradeStore) Apply(ctx context.Context, change trading.StateChange) error {
	if err := s.inner.Apply(ctx, change); err != nil {
		return err
	}
	if len(change.Trades) != 0 && change.Trades[0].ID() == "trade-000001" {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestPostgresTradeCommitCoordinatorAllowsIndependentDurableChanges(t *testing.T) {
	pool := testPostgresPool(t)
	inner, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	store := &pausedTradeStore{
		inner: inner, entered: make(chan struct{}), release: make(chan struct{}),
	}
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	service := trading.NewServiceWithStore(store)

	for _, symbol := range []string{"BTCUSD", "ETHUSD"} {
		if _, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
			ID: "rest-" + symbol, Symbol: symbol, Side: orders.SideBuy,
			Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
		}); err != nil {
			t.Fatalf("resting %s: %v", symbol, err)
		}
	}

	tradeDone := make(chan error, 1)
	go func() {
		_, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
			ID: "cross-btc", Symbol: "BTCUSD", Side: orders.SideSell,
			Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
		})
		tradeDone <- err
	}()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("trade transaction never committed")
	}

	done := make(chan error, 1)
	go func() {
		if _, err := service.CancelOrder(ctx, "rest-ETHUSD"); err != nil {
			done <- fmt.Errorf("cancel ETH: %w", err)
			return
		}
		_, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
			ID: "rest-sol", Symbol: "SOLUSD", Side: orders.SideBuy,
			Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("independent durable operations: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("independent operations waited for blocked trade commit")
	}
	btc, err := service.GetOrder("rest-BTCUSD")
	if err != nil || btc.Status != orders.OrderStatusOpen {
		t.Fatalf("unpublished trade became visible too early: %+v, err=%v", btc, err)
	}

	release()
	if err := <-tradeDone; err != nil {
		t.Fatalf("finalize committed trade: %v", err)
	}

	state, err := inner.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.TradeSequence != 1 || len(state.Orders) != 4 {
		t.Fatalf("durable sequence/orders mismatch: %d, %d",
			state.TradeSequence, len(state.Orders))
	}
	recovered, err := trading.NewRecoveredService(inner, state)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]orders.OrderStatus{
		"rest-ETHUSD": orders.OrderStatusCancelled,
		"rest-BTCUSD": orders.OrderStatusFilled,
		"cross-btc":   orders.OrderStatusFilled,
		"rest-sol":    orders.OrderStatusOpen,
	} {
		got, err := recovered.GetOrder(id)
		if err != nil || got.Status != want {
			t.Errorf("%s after recovery: %+v, err=%v, want=%v", id, got, err, want)
		}
	}
	next, err := recovered.SubmitOrder(ctx, trading.SubmitOrderInput{
		ID: "cross-sol", Symbol: "SOLUSD", Side: orders.SideSell,
		Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Trades) != 1 || next.Trades[0].ID() != "trade-000002" {
		t.Errorf("global sequence after PostgreSQL recovery: %+v", next.Trades)
	}
}
