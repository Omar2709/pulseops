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
		context.Background(),
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
		context.Background(),
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
		context.Background(),
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
		context.Background(),
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
		context.Background(),
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
		context.Background(),
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
		context.Background(),
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
		context.Background(),
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
		context.Background(),
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
				context.Background(),
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
		context.Background(),
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
		context.Background(),
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

// TestCancelOrderRemovesOpenOrderFromBook verifies that cancellation
// removes an order from the active book without removing it from the registry.
func TestCancelOrderRemovesOpenOrderFromBook(
	t *testing.T,
) {
	now := testTime()

	service := newService(func() time.Time {
		return now
	})

	created, err := service.SubmitOrder(
		context.Background(),
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

	now = now.Add(time.Second)

	cancelled, err := service.CancelOrder(" buy-001 ")
	if err != nil {
		t.Fatalf("cancelling order: %v", err)
	}

	if cancelled.Status != orders.OrderStatusCancelled {
		t.Errorf(
			"expected CANCELLED, got %s",
			cancelled.Status,
		)
	}

	if cancelled.RemainingQuantity != 5_000_000 {
		t.Errorf(
			"unexpected remaining quantity: %d",
			cancelled.RemainingQuantity,
		)
	}

	if !cancelled.UpdatedAt.Equal(now) {
		t.Errorf(
			"unexpected update time: %v",
			cancelled.UpdatedAt,
		)
	}

	book := service.books["BTCUSD"]
	if book == nil {
		t.Fatal("expected BTCUSD order book to exist")
	}

	if len(book.Bids()) != 0 {
		t.Error("cancelled order remains in order book")
	}

	stored, err := service.GetOrder("buy-001")
	if err != nil {
		t.Fatalf("retrieving cancelled order: %v", err)
	}

	if stored.Status != orders.OrderStatusCancelled {
		t.Errorf(
			"expected stored order to be CANCELLED, got %s",
			stored.Status,
		)
	}

	if created.Order.Status != orders.OrderStatusOpen {
		t.Error("original submission snapshot was modified")
	}

	_, err = service.CancelOrder("buy-001")

	if !errors.Is(err, ErrOrderNotCancellable) {
		t.Fatalf(
			"expected ErrOrderNotCancellable, got %v",
			err,
		)
	}
}

// TestCancelOrderPreservesPartialFillAndPreventsFurtherMatching verifies
// that cancelling a partially filled order preserves its execution history
// and removes its remaining quantity from active matching.
func TestCancelOrderPreservesPartialFillAndPreventsFurtherMatching(
	t *testing.T,
) {
	now := testTime()

	service := newService(func() time.Time {
		return now
	})

	// Register a SELL order for 0.05 BTC.
	_, err := service.SubmitOrder(
		context.Background(),
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

	// Execute 0.02 BTC, leaving 0.03 BTC pending.
	now = now.Add(time.Second)

	result, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(2_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting first buy: %v", err)
	}

	if len(result.Trades) != 1 {
		t.Fatalf(
			"expected 1 trade, got %d",
			len(result.Trades),
		)
	}

	before, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting sell order: %v", err)
	}

	if before.Status != orders.OrderStatusPartiallyFilled {
		t.Fatalf(
			"expected PARTIALLY_FILLED, got %s",
			before.Status,
		)
	}

	// Cancel the remaining quantity.
	now = now.Add(time.Second)

	cancelled, err := service.CancelOrder("sell-001")
	if err != nil {
		t.Fatalf("cancelling partial order: %v", err)
	}

	if cancelled.Status != orders.OrderStatusCancelled {
		t.Errorf(
			"expected CANCELLED, got %s",
			cancelled.Status,
		)
	}

	if cancelled.FilledQuantity != orders.Quantity(2_000_000) {
		t.Errorf(
			"expected filled quantity 2000000, got %d",
			cancelled.FilledQuantity,
		)
	}

	if cancelled.RemainingQuantity != orders.Quantity(3_000_000) {
		t.Errorf(
			"expected remaining quantity 3000000, got %d",
			cancelled.RemainingQuantity,
		)
	}

	book := service.books["BTCUSD"]
	if book == nil {
		t.Fatal("expected BTCUSD order book to exist")
	}

	if len(book.Asks()) != 0 {
		t.Fatal("cancelled SELL remains in the order book")
	}

	// A new BUY must not execute against the cancelled SELL.
	now = now.Add(time.Second)

	next, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "buy-002",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(3_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting second buy: %v", err)
	}

	if len(next.Trades) != 0 {
		t.Errorf(
			"expected 0 trades after cancellation, got %d",
			len(next.Trades),
		)
	}

	if next.Order.Status != orders.OrderStatusOpen {
		t.Errorf(
			"expected new BUY to remain OPEN, got %s",
			next.Order.Status,
		)
	}

	stored, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting cancelled order: %v", err)
	}

	if stored.Status != orders.OrderStatusCancelled {
		t.Error("cancelled order changed status unexpectedly")
	}

	if stored.FilledQuantity != orders.Quantity(2_000_000) {
		t.Error("historical executed quantity changed unexpectedly")
	}
}

// TestCancelOrderRejectsFilledOrder verifies that a terminal FILLED
// order cannot transition to CANCELLED.
func TestCancelOrderRejectsFilledOrder(t *testing.T) {
	now := testTime()

	service := newService(func() time.Time {
		return now
	})

	_, err := service.SubmitOrder(
		context.Background(),
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
		context.Background(),
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

	now = now.Add(time.Second)

	_, err = service.CancelOrder("sell-001")

	if !errors.Is(err, ErrOrderNotCancellable) {
		t.Fatalf(
			"expected ErrOrderNotCancellable, got %v",
			err,
		)
	}

	stored, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("retrieving filled order: %v", err)
	}

	if stored.Status != orders.OrderStatusFilled {
		t.Errorf(
			"expected FILLED, got %s",
			stored.Status,
		)
	}

	if stored.RemainingQuantity != 0 {
		t.Error("filled order has unexpected remaining quantity")
	}
}

// TestCancelOrderReturnsNotFound verifies that an unknown ID
// returns the application's not-found error.
func TestCancelOrderReturnsNotFound(t *testing.T) {
	service := NewService()

	_, err := service.CancelOrder("unknown-order")

	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf(
			"expected ErrOrderNotFound, got %v",
			err,
		)
	}
}

// TestSubmitOrderMatchingFailureDoesNotChangeState verifies that
// failed matching does not modify previously committed state.
func TestSubmitOrderMatchingFailureDoesNotChangeState(
	t *testing.T,
) {
	// Create a resting SELL order.
	now := testTime().Add(2 * time.Second)

	service := newService(func() time.Time {
		return now
	})

	_, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "sell-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting initial sell: %v", err)
	}

	before, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting initial order: %v", err)
	}

	// Simulate the clock moving backwards.
	// Matching at this earlier timestamp must fail.
	now = now.Add(-time.Second)

	_, err = service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)

	if !errors.Is(
		err,
		orders.ErrOrderUpdateTimeBeforeCurrent,
	) {
		t.Fatalf(
			"expected matching timestamp error, got %v",
			err,
		)
	}

	// The existing order must remain unchanged.
	after, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting existing order: %v", err)
	}

	if after != before {
		t.Error("existing order changed after failed matching")
	}

	// The rejected incoming order must not be registered.
	_, err = service.GetOrder("buy-001")

	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf(
			"expected incoming order to be absent, got %v",
			err,
		)
	}

	// The incoming order must not remain in the book.
	book := service.books["BTCUSD"]
	if book == nil {
		t.Fatal("expected BTCUSD order book")
	}

	if len(book.Bids()) != 0 {
		t.Error("failed incoming BUY remains in the order book")
	}

	if len(book.Asks()) != 1 {
		t.Error("existing SELL was unexpectedly removed")
	}

	if len(service.orders) != 1 {
		t.Errorf(
			"expected 1 registered order, got %d",
			len(service.orders),
		)
	}
}

// TestSubmitOrderFailureAfterFirstTradeDoesNotChangeState verifies
// that a later matching failure discards earlier provisional fills.
func TestSubmitOrderFailureAfterFirstTradeDoesNotChangeState(
	t *testing.T,
) {
	start := testTime()
	now := start

	service := newService(func() time.Time {
		return now
	})

	// First SELL: created at T0.
	_, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "sell-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(2_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting first sell: %v", err)
	}

	// Second SELL: created at T0 + 2 seconds.
	now = start.Add(2 * time.Second)

	_, err = service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "sell-002",
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(3_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting second sell: %v", err)
	}

	firstBefore, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting first sell: %v", err)
	}

	secondBefore, err := service.GetOrder("sell-002")
	if err != nil {
		t.Fatalf("getting second sell: %v", err)
	}

	// Simulate a clock adjustment.
	//
	// The incoming BUY can match the first SELL at T0 + 1,
	// but the second SELL was created later, at T0 + 2.
	now = start.Add(time.Second)

	_, err = service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_200_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)

	if !errors.Is(
		err,
		orders.ErrOrderUpdateTimeBeforeCurrent,
	) {
		t.Fatalf(
			"expected matching timestamp error, got %v",
			err,
		)
	}

	// Neither existing order may change after failure.
	firstAfter, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting first sell after failure: %v", err)
	}

	secondAfter, err := service.GetOrder("sell-002")
	if err != nil {
		t.Fatalf("getting second sell after failure: %v", err)
	}

	if firstAfter != firstBefore {
		t.Error("first SELL changed after failed submission")
	}

	if secondAfter != secondBefore {
		t.Error("second SELL changed after failed submission")
	}

	// The failed incoming order must not exist.
	_, err = service.GetOrder("buy-001")

	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf(
			"expected incoming BUY to be absent, got %v",
			err,
		)
	}

	// Both original SELL orders must remain active.
	book := service.books["BTCUSD"]
	if book == nil {
		t.Fatal("expected BTCUSD order book")
	}

	if got := len(book.Asks()); got != 2 {
		t.Errorf(
			"expected 2 original asks, got %d",
			got,
		)
	}

	if got := len(book.Bids()); got != 0 {
		t.Errorf(
			"expected 0 bids after failure, got %d",
			got,
		)
	}

	if got := len(service.orders); got != 2 {
		t.Errorf(
			"expected 2 registered orders, got %d",
			got,
		)
	}
}

// TestSubmitOrderFailureDoesNotConsumeTradeSequence verifies that a failed
// submission does not commit provisional trades or advance the trade ID sequence.
func TestSubmitOrderFailureDoesNotConsumeTradeSequence(
	t *testing.T,
) {
	start := testTime()
	now := start

	service := newService(func() time.Time {
		return now
	})

	submit := func(
		id string,
		side orders.Side,
		price orders.Price,
		quantity orders.Quantity,
	) (SubmitOrderResult, error) {
		t.Helper()

		return service.SubmitOrder(
			context.Background(),
			SubmitOrderInput{
				ID:       id,
				Symbol:   "BTCUSD",
				Side:     side,
				Price:    price,
				Quantity: quantity,
			},
		)
	}

	// Step 1: execute an initial valid trade.
	_, err := submit(
		"seed-sell",
		orders.SideSell,
		orders.Price(6_000_000),
		orders.Quantity(1_000_000),
	)
	if err != nil {
		t.Fatalf("submitting initial SELL: %v", err)
	}

	now = start.Add(time.Second)

	initial, err := submit(
		"seed-buy",
		orders.SideBuy,
		orders.Price(6_000_000),
		orders.Quantity(1_000_000),
	)
	if err != nil {
		t.Fatalf("submitting initial BUY: %v", err)
	}

	if len(initial.Trades) != 1 {
		t.Fatalf(
			"expected 1 initial trade, got %d",
			len(initial.Trades),
		)
	}

	if got := initial.Trades[0].ID(); got != "trade-000001" {
		t.Fatalf(
			"expected initial trade-000001, got %q",
			got,
		)
	}

	// Step 2: register two SELL orders at different times.
	now = start.Add(3 * time.Second)

	_, err = submit(
		"sell-001",
		orders.SideSell,
		orders.Price(6_000_000),
		orders.Quantity(2_000_000),
	)
	if err != nil {
		t.Fatalf("submitting first SELL: %v", err)
	}

	now = start.Add(5 * time.Second)

	_, err = submit(
		"sell-002",
		orders.SideSell,
		orders.Price(6_100_000),
		orders.Quantity(3_000_000),
	)
	if err != nil {
		t.Fatalf("submitting second SELL: %v", err)
	}

	// Step 3: move the clock backwards.
	//
	// The first match can succeed, but the second must fail.
	// Neither state changes nor trade IDs should be committed.
	now = start.Add(4 * time.Second)

	_, err = submit(
		"failed-buy",
		orders.SideBuy,
		orders.Price(6_200_000),
		orders.Quantity(5_000_000),
	)

	if !errors.Is(
		err,
		orders.ErrOrderUpdateTimeBeforeCurrent,
	) {
		t.Fatalf(
			"expected matching timestamp error, got %v",
			err,
		)
	}

	// Step 4: submit a valid BUY after the failed attempt.
	//
	// Both original SELL orders should still be available.
	now = start.Add(6 * time.Second)

	result, err := submit(
		"valid-buy",
		orders.SideBuy,
		orders.Price(6_200_000),
		orders.Quantity(5_000_000),
	)
	if err != nil {
		t.Fatalf("submitting valid BUY: %v", err)
	}

	if len(result.Trades) != 2 {
		t.Fatalf(
			"expected 2 trades after rollback, got %d",
			len(result.Trades),
		)
	}

	expectedIDs := []string{
		"trade-000002",
		"trade-000003",
	}

	for index, expected := range expectedIDs {
		if got := result.Trades[index].ID(); got != expected {
			t.Errorf(
				"trade %d: expected ID %q, got %q",
				index,
				expected,
				got,
			)
		}
	}
}
