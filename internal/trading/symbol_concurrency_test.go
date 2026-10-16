package trading

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

// Both symbols must be able to reach the staging clock before either call
// returns. A single global lock around matching would block the second call.
func TestSubmitOrderStagesDifferentSymbolsInParallel(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	service := newService(func() time.Time {
		entered <- struct{}{}
		<-release
		return testTime()
	})

	type result struct{ err error }
	finished := make(chan result, 2)
	for _, symbol := range []string{"BTCUSD", "ETHUSD"} {
		symbol := symbol
		go func() {
			_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
				ID:       "buy-" + symbol,
				Symbol:   symbol,
				Side:     orders.SideBuy,
				Price:    orders.Price(10),
				Quantity: orders.Quantity(1),
			})
			finished <- result{err: err}
		}()
	}

	var reached int
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for reached < 2 {
		select {
		case <-entered:
			reached++
		case <-timer.C:
			close(release)
			for i := 0; i < 2; i++ {
				<-finished
			}
			t.Fatal("separate symbols could not stage concurrently")
		}
	}
	close(release)

	for i := 0; i < 2; i++ {
		if result := <-finished; result.err != nil {
			t.Errorf("submitting parallel order: %v", result.err)
		}
	}
}

func TestSubmitOrderConcurrentSymbolsPreserveGlobalTradeSequence(t *testing.T) {
	service := NewService()
	for _, symbol := range []string{"BTCUSD", "ETHUSD", "SOLUSD"} {
		_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
			ID:       "sell-" + symbol,
			Symbol:   symbol,
			Side:     orders.SideSell,
			Price:    orders.Price(100),
			Quantity: orders.Quantity(1),
		})
		if err != nil {
			t.Fatalf("creating resting order %s: %v", symbol, err)
		}
	}

	type result struct {
		symbol string
		trades []*orders.Trade
		err    error
	}
	results := make(chan result, 3)
	var wg sync.WaitGroup
	for _, symbol := range []string{"BTCUSD", "ETHUSD", "SOLUSD"} {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			res, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
				ID:       "buy-" + symbol,
				Symbol:   symbol,
				Side:     orders.SideBuy,
				Price:    orders.Price(101),
				Quantity: orders.Quantity(1),
			})
			results <- result{symbol: symbol, trades: res.Trades, err: err}
		}(symbol)
	}
	wg.Wait()
	close(results)

	seen := map[string]bool{}
	for r := range results {
		if r.err != nil {
			t.Fatalf("matching %s: %v", r.symbol, r.err)
		}
		if len(r.trades) != 1 {
			t.Fatalf("expected exactly one trade on %s, got %d", r.symbol, len(r.trades))
		}
		trade := r.trades[0]
		if trade.Symbol() != r.symbol {
			t.Errorf("cross-symbol execution: %s matched %s", r.symbol, trade.Symbol())
		}
		if seen[trade.ID()] {
			t.Errorf("duplicate trade ID %s", trade.ID())
		}
		seen[trade.ID()] = true
	}
	for i := 1; i <= 3; i++ {
		if !seen[fmt.Sprintf("trade-%06d", i)] {
			t.Errorf("missing global trade sequence ID %d", i)
		}
	}
	if service.engine.TradeSequence() != 3 {
		t.Errorf("expected confirmed trade sequence 3, got %d", service.engine.TradeSequence())
	}
}

func TestSubmitOrderConcurrentSameIDOnDifferentSymbols(t *testing.T) {
	service := NewService()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, symbol := range []string{"BTCUSD", "ETHUSD"} {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
				ID:       "shared-id",
				Symbol:   symbol,
				Side:     orders.SideBuy,
				Price:    orders.Price(100),
				Quantity: orders.Quantity(1),
			})
			results <- err
		}(symbol)
	}
	wg.Wait()
	close(results)

	var created, conflicts int
	for err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrDuplicateOrder):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent submission error: %v", err)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Errorf("expected one order and one conflict, got %d and %d", created, conflicts)
	}
	if len(service.orders) != 1 {
		t.Errorf("expected one globally unique order ID, got %d", len(service.orders))
	}
}

func TestConcurrentCancelAndSubmitSameSymbolPreserveBook(t *testing.T) {
	service := NewService()
	_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
		ID:       "sell-001",
		Symbol:   "BTCUSD",
		Side:     orders.SideSell,
		Price:    orders.Price(100),
		Quantity: orders.Quantity(1),
	})
	if err != nil {
		t.Fatalf("creating resting SELL: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	go func() {
		defer wg.Done()
		_, err := service.CancelOrder(context.Background(), "sell-001")
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, err := service.SubmitOrder(context.Background(), SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(101),
			Quantity: orders.Quantity(1),
		})
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, ErrOrderNotCancellable) {
			t.Fatalf("unexpected concurrent cancel/submit error: %v", err)
		}
	}

	sell, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting SELL: %v", err)
	}
	book := service.books["BTCUSD"]
	if sell.Status == orders.OrderStatusCancelled {
		if got := len(book.Asks()); got != 0 {
			t.Errorf("cancelled SELL remains in book: %d asks", got)
		}
	} else if sell.Status != orders.OrderStatusFilled {
		t.Errorf("unexpected SELL status %s", sell.Status)
	}
}
