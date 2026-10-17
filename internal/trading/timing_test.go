package trading

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Omar2709/pulseops/internal/orders"
)

type timingCollector struct {
	mu   sync.Mutex
	data []OperationTiming
}

func (c *timingCollector) Observe(value OperationTiming) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = append(c.data, value)
}

func (c *timingCollector) snapshot() []OperationTiming {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]OperationTiming(nil), c.data...)
}

func TestTimingObserverReportsSuccessReplayFailureAndCancellation(t *testing.T) {
	collector := &timingCollector{}
	service := NewServiceWithStoreAndTimingObserver(noopStateStore{}, collector)
	sell := SubmitOrderInput{
		ID: "sell", Symbol: "BTCUSD", Side: orders.SideSell,
		Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
	}

	if _, err := service.SubmitOrder(context.Background(), sell); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitOrder(context.Background(), sell); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID: "buy", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
	}); err != nil {
		t.Fatal(err)
	}

	service.store = &recordingStateStore{err: errors.New("store offline")}
	failed := sell
	failed.ID = "sell-failed"
	if _, err := service.SubmitOrder(context.Background(), failed); err == nil {
		t.Error("expected persistence failure")
	}
	service.store = noopStateStore{}

	remaining := sell
	remaining.ID = "sell-remaining"
	if _, err := service.SubmitOrder(context.Background(), remaining); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CancelOrder(context.Background(), remaining.ID); err != nil {
		t.Fatal(err)
	}

	got := collector.snapshot()
	outcomes := []TimingOutcome{
		TimingCommitted, TimingReplayed, TimingCommitted,
		TimingRejected, TimingCommitted, TimingCommitted,
	}
	if len(got) != len(outcomes) {
		t.Fatalf("expected %d observations, got %d: %+v", len(outcomes), len(got), got)
	}
	for i, entry := range got {
		wantOperation := TimingSubmit
		if i == len(got)-1 {
			wantOperation = TimingCancel
		}
		if entry.Operation != wantOperation || entry.Outcome != outcomes[i] {
			t.Errorf("observation %d: got %+v", i, entry)
		}
		if entry.SymbolLockWait < 0 || entry.GlobalLockWait < 0 ||
			entry.StoreApply < 0 || entry.Publication < 0 {
			t.Errorf("observation %d has negative duration: %+v", i, entry)
		}
	}
	if got[1].StoreApply != 0 || got[1].Publication != 0 {
		t.Errorf("idempotent replay should not persist or publish: %+v", got[1])
	}
	if got[3].Publication != 0 {
		t.Errorf("failed store write should not publish: %+v", got[3])
	}
	if got[2].StoreApply == 0 || got[5].StoreApply == 0 {
		t.Error("matched trade and cancellation should measure Store.Apply")
	}
}

func TestTimingObserverCalledAfterServiceLocksReleased(t *testing.T) {
	var service *Service
	observer := TimingObserverFunc(func(_ OperationTiming) {
		if _, err := service.GetOrder("buy"); err != nil {
			t.Errorf("cannot look up order from observer: %v", err)
		}
		symbolGate := service.symbolLock("BTCUSD")
		if !symbolGate.TryLock() {
			t.Error("observer invoked while per-symbol lock is held")
			return
		}
		symbolGate.Unlock()
	})
	service = NewServiceWithStoreAndTimingObserver(noopStateStore{}, observer)
	if _, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID: "buy", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTimingObserverConcurrentCalls(t *testing.T) {
	collector := &timingCollector{}
	service := NewServiceWithStoreAndTimingObserver(noopStateStore{}, collector)
	const total = 48
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
				ID: fmt.Sprintf("buy-%d", i),
				Symbol: fmt.Sprintf("ASSET%dUSD", i%8),
				Side: orders.SideBuy,
				Price: orders.Price(10_000),
				Quantity: orders.Quantity(1_000),
			})
			if err != nil {
				t.Errorf("submit %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if got := len(collector.snapshot()); got != total {
		t.Fatalf("expected %d timing observations, got %d", total, got)
	}
}

func TestTimingObserverPreservesContextCancellation(t *testing.T) {
	collector := &timingCollector{}
	service := NewServiceWithStoreAndTimingObserver(noopStateStore{}, collector)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.SubmitOrder(ctx, SubmitOrderInput{
		ID: "buy", Symbol: "BTCUSD", Side: orders.SideBuy,
		Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	events := collector.snapshot()
	if len(events) != 1 || events[0].Outcome != TimingRejected {
		t.Errorf("cancelled submission should be rejected: %+v", events)
	}
}
