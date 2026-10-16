package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
)

type recordingPostgresStore struct {
	store   *Store
	changes []trading.StateChange
}

func (r *recordingPostgresStore) Apply(ctx context.Context, c trading.StateChange) error {
	if err := r.store.Apply(ctx, c); err != nil {
		return err
	}
	r.changes = append(r.changes, c)
	return nil
}

func TestRestingOrderFastPathPreservesPostgresRecoveryFIFO(t *testing.T) {
	pool := testPostgresPool(t)
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingPostgresStore{store: store}
	service := trading.NewServiceWithStore(recorder)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, id := range []string{"sell-first", "sell-second"} {
		_, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
			ID: id, Symbol: "BTCUSD", Side: orders.SideSell,
			Price: orders.Price(6_000_000), Quantity: orders.Quantity(1_000_000),
		})
		if err != nil {
			t.Fatalf("submit %s: %v", id, err)
		}
	}
	if len(recorder.changes) != 2 {
		t.Fatalf("expected two committed changes, got %d", len(recorder.changes))
	}
	for index, change := range recorder.changes {
		if len(change.Orders) != 1 || len(change.BookSequences) != 1 {
			t.Errorf("change %d re-persisted unchanged orders: %+v", index, change)
		}
	}
	state, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Orders) != 2 {
		t.Fatalf("expected both orders after recovery, got %d", len(state.Orders))
	}
	recovered, err := trading.NewRecoveredService(store, state)
	if err != nil {
		t.Fatal(err)
	}

	buy, err := recovered.SubmitOrder(ctx, trading.SubmitOrderInput{
		ID: "buy-after-recovery", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(6_000_000), Quantity: orders.Quantity(1_000_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(buy.Trades) != 1 || buy.Trades[0].SellOrderID() != "sell-first" {
		t.Fatalf("first restored FIFO sell was not matched: %+v", buy.Trades)
	}
}
