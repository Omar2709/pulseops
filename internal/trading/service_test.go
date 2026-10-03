package trading

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

func testTime() time.Time {
	return time.Date(
		2026,
		time.September,
		26,
		14,
		0,
		0,
		0,
		time.UTC,
	)
}

func TestSubmitOrderAddsOpenOrder(t *testing.T) {
	now := testTime()
	service := newService(func() time.Time {
		return now
	})

	result, err := service.SubmitOrder(
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)

	if err != nil {
		t.Fatalf("submitting order: %v", err)
	}

	if result.Order.Status != orders.OrderStatusOpen {
		t.Errorf(
			"expected status %q, got %q",
			orders.OrderStatusOpen,
			result.Order.Status,
		)
	}

	if len(result.Trades) != 0 {
		t.Errorf(
			"expected 0 trades, got %d",
			len(result.Trades),
		)
	}
}

func TestSubmitOrderMatchesCrossingOrders(t *testing.T) {
	now := testTime()
	service := newService(func() time.Time {
		return now
	})

	_, err := service.SubmitOrder(
		SubmitOrderInput{
			ID:       "sell-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting sell order: %v", err)
	}

	now = now.Add(time.Second)

	result, err := service.SubmitOrder(
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)

	if err != nil {
		t.Fatalf("submitting buy order: %v", err)
	}

	if len(result.Trades) != 1 {
		t.Fatalf(
			"expected 1 trade, got %d",
			len(result.Trades),
		)
	}

	if result.Order.Status != orders.OrderStatusFilled {
		t.Errorf(
			"expected order status %q, got %q",
			orders.OrderStatusFilled,
			result.Order.Status,
		)
	}
}

func TestSubmitOrderRejectsDuplicateIDWithoutChangingState(
	t *testing.T,
) {
	now := testTime()
	service := newService(func() time.Time {
		return now
	})

	_, err := service.SubmitOrder(
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting first order: %v", err)
	}

	now = now.Add(time.Second)

	result, err := service.SubmitOrder(
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)

	if !errors.Is(err, ErrDuplicateOrder) {
		t.Fatalf(
			"expected ErrDuplicateOrder, got %v",
			err,
		)
	}

	if len(result.Trades) != 0 {
		t.Errorf(
			"expected duplicate submission to produce 0 trades, got %d",
			len(result.Trades),
		)
	}

	if len(service.orders) != 1 {
		t.Errorf(
			"expected 1 registered order, got %d",
			len(service.orders),
		)
	}

	book := service.books["BTCUSD"]
	if book == nil {
		t.Fatal("expected BTCUSD order book to exist")
	}

	if len(book.Bids()) != 1 {
		t.Errorf(
			"expected 1 bid after duplicate rejection, got %d",
			len(book.Bids()),
		)
	}
}

func TestSubmitOrderUsesIndependentBooksPerSymbol(
	t *testing.T,
) {
	now := testTime()
	service := newService(func() time.Time {
		return now
	})

	_, err := service.SubmitOrder(
		SubmitOrderInput{
			ID:       "sell-btc-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(100),
			Quantity: orders.Quantity(1),
		},
	)
	if err != nil {
		t.Fatalf("submitting BTC sell order: %v", err)
	}

	now = now.Add(time.Second)

	result, err := service.SubmitOrder(
		SubmitOrderInput{
			ID:       "buy-eth-001",
			Symbol:   "ETHUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(200),
			Quantity: orders.Quantity(1),
		},
	)
	if err != nil {
		t.Fatalf("submitting ETH buy order: %v", err)
	}

	if len(result.Trades) != 0 {
		t.Errorf(
			"expected 0 trades across different symbols, got %d",
			len(result.Trades),
		)
	}

	if result.Order.Status != orders.OrderStatusOpen {
		t.Errorf(
			"expected ETH order status %q, got %q",
			orders.OrderStatusOpen,
			result.Order.Status,
		)
	}
}

func TestSubmitOrderKeepsFilledOrdersInRegistryAndRemovesThemFromBook(
	t *testing.T,
) {
	now := testTime()
	service := newService(func() time.Time {
		return now
	})

	_, err := service.SubmitOrder(
		SubmitOrderInput{
			ID:       "sell-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting sell order: %v", err)
	}

	now = now.Add(time.Second)

	_, err = service.SubmitOrder(
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting buy order: %v", err)
	}

	buy := service.orders["buy-001"]
	if buy == nil {
		t.Fatal("expected buy order to remain registered")
	}

	sell := service.orders["sell-001"]
	if sell == nil {
		t.Fatal("expected sell order to remain registered")
	}

	if buy.Status() != orders.OrderStatusFilled {
		t.Errorf(
			"expected buy order status %q, got %q",
			orders.OrderStatusFilled,
			buy.Status(),
		)
	}

	if sell.Status() != orders.OrderStatusFilled {
		t.Errorf(
			"expected sell order status %q, got %q",
			orders.OrderStatusFilled,
			sell.Status(),
		)
	}

	book := service.books["BTCUSD"]
	if book == nil {
		t.Fatal("expected BTCUSD order book to exist")
	}

	if len(book.Bids()) != 0 || len(book.Asks()) != 0 {
		t.Fatal(
			"expected filled orders to be removed from the order book",
		)
	}
}

func TestSubmitOrderConcurrentRequests(t *testing.T) {
	service := NewService()

	const total = 32

	var wg sync.WaitGroup
	errs := make(chan error, total)

	for i := 0; i < total; i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			_, err := service.SubmitOrder(
				SubmitOrderInput{
					ID: fmt.Sprintf(
						"buy-%03d",
						index,
					),
					Symbol:   "BTCUSD",
					Side:     orders.SideBuy,
					Price:    orders.Price(6_000_000),
					Quantity: orders.Quantity(5_000_000),
				},
			)

			errs <- err
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent submission: %v", err)
		}
	}

	if got := len(service.orders); got != total {
		t.Errorf(
			"expected %d registered orders, got %d",
			total,
			got,
		)
	}

	book := service.books["BTCUSD"]
	if book == nil {
		t.Fatal("expected BTCUSD order book")
	}

	if got := len(book.Bids()); got != total {
		t.Errorf(
			"expected %d bids, got %d",
			total,
			got,
		)
	}
}

// Verify that GetOrder returns the current state after matching
// and that snapshots cannot mutate the stored order.
func TestGetOrderReturnsCurrentSnapshot(t *testing.T) {
	now := testTime()

	service := newService(func() time.Time {
		return now
	})

	_, err := service.SubmitOrder(
		SubmitOrderInput{
			ID:       "sell-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting sell order: %v", err)
	}

	// Retrieve the order before matching.
	initial, err := service.GetOrder(" sell-001 ")
	if err != nil {
		t.Fatalf("getting initial order: %v", err)
	}

	if initial.Status != orders.OrderStatusOpen {
		t.Fatalf(
			"expected OPEN, got %s",
			initial.Status,
		)
	}

	// Submit a compatible BUY order.
	now = now.Add(time.Second)

	_, err = service.SubmitOrder(
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting buy order: %v", err)
	}

	// Retrieve the previously registered SELL order.
	current, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting updated order: %v", err)
	}

	if current.Status != orders.OrderStatusFilled {
		t.Errorf(
			"expected FILLED, got %s",
			current.Status,
		)
	}

	if current.FilledQuantity != orders.Quantity(5_000_000) {
		t.Errorf(
			"unexpected filled quantity: %d",
			current.FilledQuantity,
		)
	}

	if current.RemainingQuantity != 0 {
		t.Errorf(
			"expected no remaining quantity, got %d",
			current.RemainingQuantity,
		)
	}

	// The previous snapshot must remain unchanged.
	if initial.Status != orders.OrderStatusOpen {
		t.Error("initial snapshot was unexpectedly modified")
	}

	// A caller must not be able to mutate stored state.
	current.Status = orders.OrderStatusCancelled

	unchanged, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting stored order: %v", err)
	}

	if unchanged.Status != orders.OrderStatusFilled {
		t.Error("stored order was modified through a snapshot")
	}
}

// Verify that an unknown ID produces the expected application error.
func TestGetOrderReturnsNotFound(t *testing.T) {
	service := NewService()

	_, err := service.GetOrder("unknown-order")

	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf(
			"expected ErrOrderNotFound, got %v",
			err,
		)
	}
}
