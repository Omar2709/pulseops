package trading

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

func TestNoopStateStoreRespectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := (noopStateStore{}).Apply(ctx, StateChange{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSubmitOrderCancelledContextDoesNotPublishState(t *testing.T) {
	service := newService(func() time.Time { return testTime() })

	_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID:       "sell-001",
		Symbol:   "BTCUSD",
		Side:     orders.SideSell,
		Price:    orders.Price(6_000_000),
		Quantity: orders.Quantity(1_000_000),
	})
	if err != nil {
		t.Fatalf("submitting resting SELL: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := service.SubmitOrder(ctx, SubmitOrderInput{
		ID:       "buy-cancelled",
		Symbol:   "BTCUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(6_100_000),
		Quantity: orders.Quantity(1_000_000),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if result.Order.ID != "" || len(result.Trades) != 0 {
		t.Fatalf("cancelled submission returned committed state: %+v", result)
	}

	if _, err := service.GetOrder("buy-cancelled"); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("cancelled BUY was published: %v", err)
	}
	sell, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting resting SELL: %v", err)
	}
	if sell.Status != orders.OrderStatusOpen || sell.FilledQuantity != 0 {
		t.Errorf("resting SELL was changed: %+v", sell)
	}
	if got := len(service.books["BTCUSD"].Asks()); got != 1 {
		t.Errorf("expected one resting ask, got %d", got)
	}
	if got := service.engine.TradeSequence(); got != 0 {
		t.Errorf("trade sequence advanced after cancellation: %d", got)
	}

	next, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID:       "buy-valid",
		Symbol:   "BTCUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(6_100_000),
		Quantity: orders.Quantity(1_000_000),
	})
	if err != nil {
		t.Fatalf("submitting valid BUY: %v", err)
	}
	if len(next.Trades) != 1 || next.Trades[0].ID() != "trade-000001" {
		t.Errorf("expected first trade after cancelled request, got %v", next.Trades)
	}
}

func TestCancelOrderCancelledContextDoesNotPublishState(t *testing.T) {
	service := newService(func() time.Time { return testTime() })

	_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID:       "buy-001",
		Symbol:   "BTCUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(6_000_000),
		Quantity: orders.Quantity(1_000_000),
	})
	if err != nil {
		t.Fatalf("submitting BUY: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = service.CancelOrder(ctx, "buy-001")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	current, err := service.GetOrder("buy-001")
	if err != nil {
		t.Fatalf("getting BUY: %v", err)
	}
	if current.Status != orders.OrderStatusOpen {
		t.Errorf("cancelled request changed order status to %s", current.Status)
	}
	if got := len(service.books["BTCUSD"].Bids()); got != 1 {
		t.Errorf("cancelled request removed resting bid: got %d", got)
	}
}

func TestCancelledContextDoesNotReplayExistingOrder(t *testing.T) {
	service := newService(func() time.Time { return testTime() })
	input := SubmitOrderInput{
		ID:       "existing-order",
		Symbol:   "BTCUSD",
		Side:     orders.SideBuy,
		Price:    orders.Price(6_000_000),
		Quantity: orders.Quantity(1_000_000),
	}
	if _, err := service.SubmitOrder(context.Background(), input); err != nil {
		t.Fatalf("submitting original order: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := service.SubmitOrder(ctx, input)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled request to fail, got result=%+v err=%v", result, err)
	}
	if result.Replayed || result.Order.ID != "" {
		t.Errorf("cancelled request unexpectedly returned successful replay: %+v", result)
	}
	if got := len(service.orders); got != 1 {
		t.Errorf("cancelled replay changed the registry: %d orders", got)
	}
}
