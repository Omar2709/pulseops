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

// TestTradingServiceConcurrentMatchingRecoversAllTrades exercises the whole
// service/persistence path with multiple symbols and a shared global counter.
func TestTradingServiceConcurrentMatchingRecoversAllTrades(t *testing.T) {
	pool := testPostgresPool(t)
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	service := trading.NewServiceWithStore(store)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	const symbols = 8
	const pairsPerSymbol = 4
	const totalPairs = symbols * pairsPerSymbol

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, totalPairs)
	for symbolNumber := 0; symbolNumber < symbols; symbolNumber++ {
		symbol := fmt.Sprintf("ASSET%dUSD", symbolNumber)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < pairsPerSymbol; i++ {
				sellID := fmt.Sprintf("sell-%s-%d", symbol, i)
				buyID := fmt.Sprintf("buy-%s-%d", symbol, i)
				_, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
					ID: sellID, Symbol: symbol, Side: orders.SideSell,
					Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
				})
				if err != nil {
					errs <- fmt.Errorf("%s: submit sell: %w", symbol, err)
					return
				}
				result, err := service.SubmitOrder(ctx, trading.SubmitOrderInput{
					ID: buyID, Symbol: symbol, Side: orders.SideBuy,
					Price: orders.Price(10_000), Quantity: orders.Quantity(1_000),
				})
				if err != nil {
					errs <- fmt.Errorf("%s: match buy: %w", symbol, err)
					return
				}
				if len(result.Trades) != 1 ||
					result.Trades[0].BuyOrderID() != buyID ||
					result.Trades[0].SellOrderID() != sellID {
					errs <- fmt.Errorf("%s: unexpected trade %+v", symbol, result.Trades)
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	var trades, filledOrders int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM trades").Scan(&trades); err != nil {
		t.Fatal(err)
	}
	if trades != totalPairs {
		t.Errorf("expected %d persisted trades, got %d", totalPairs, trades)
	}
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM orders WHERE status = 'FILLED'",
	).Scan(&filledOrders); err != nil {
		t.Fatal(err)
	}
	if filledOrders != 2*totalPairs {
		t.Errorf("expected %d filled orders, got %d", 2*totalPairs, filledOrders)
	}

	state, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.TradeSequence != totalPairs || len(state.Orders) != 2*totalPairs {
		t.Fatalf("recovered %d orders at sequence %d; want %d at %d",
			len(state.Orders), state.TradeSequence, 2*totalPairs, totalPairs)
	}
	if _, err := trading.NewRecoveredService(store, state); err != nil {
		t.Fatalf("rebuild service after concurrent trades: %v", err)
	}
}
