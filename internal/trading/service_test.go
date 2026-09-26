package trading

import (
	"errors"
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

func TestSubmitOrderRejectsDuplicateIDWithoutChangingState(t *testing.T) {
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

func TestSubmitOrderUsesIndependentBooksPerSymbol(t *testing.T) {
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

func TestSubmitOrderKeepsFilledOrdersInRegistryAndRemovesThemFromBook(t *testing.T) {
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
		t.Fatal("expected filled orders to be removed from the order book")
	}
}
