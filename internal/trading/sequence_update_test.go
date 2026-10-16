package trading

import (
	"context"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

func TestSequenceUpdateOnlyWhenTradeIsGenerated(t *testing.T) {
	service := newService(func() time.Time { return testTime() })
	recorder := &recordingStateStore{}
	service.store = recorder

	first := SubmitOrderInput{
		ID: "sell-1", Symbol: "BTCUSD", Side: orders.SideSell,
		Price: orders.Price(6_000_000), Quantity: orders.Quantity(1_000_000),
	}
	if _, err := service.SubmitOrder(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if !recorder.change.SkipTradeSequenceUpdate {
		t.Error("resting insert should not update global trade sequence")
	}

	buy := SubmitOrderInput{
		ID: "buy-1", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(6_000_000), Quantity: orders.Quantity(1_000_000),
	}
	result, err := service.SubmitOrder(context.Background(), buy)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || len(result.Trades) != 1 {
		t.Fatalf("expected matching trade, got %+v", result)
	}
	if recorder.change.SkipTradeSequenceUpdate || recorder.change.TradeSequence != 1 {
		t.Error("matching submission must persist global trade sequence")
	}

	_, err = service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID: "sell-2", Symbol: "BTCUSD", Side: orders.SideSell,
		Price: orders.Price(6_000_000), Quantity: orders.Quantity(1_000_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !recorder.change.SkipTradeSequenceUpdate || recorder.change.TradeSequence != 1 {
		t.Error("resting insert must preserve previously confirmed trade sequence")
	}
	if _, err := service.CancelOrder(context.Background(), "sell-2"); err != nil {
		t.Fatal(err)
	}
	if !recorder.change.SkipTradeSequenceUpdate || recorder.change.TradeSequence != 1 {
		t.Error("cancellation must not rewrite existing trade sequence")
	}
}
