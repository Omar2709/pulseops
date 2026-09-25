package orders

import (
	"errors"

	"testing"

	"time"
)

func newOpenOrderForMatching(

	t *testing.T,

	id string,

	symbol string,

	side Side,

	price Price,

	quantity Quantity,

) *Order {

	t.Helper()

	createdAt := time.Date(

		2026,

		time.September,

		25,

		12,

		0,

		0,

		0,

		time.UTC,
	)

	order, err := NewOrder(

		id,

		symbol,

		side,

		price,

		quantity,

		createdAt,
	)

	if err != nil {

		t.Fatalf(

			"failed to create order: %v",

			err,
		)

	}

	if err := order.Open(

		createdAt.Add(time.Second),
	); err != nil {

		t.Fatalf(

			"failed to open order: %v",

			err,
		)

	}

	return order

}

func TestMatchingEngineDoesNotMatchUncrossedBook(t *testing.T) {

	book := NewOrderBook()

	engine := NewMatchingEngine()

	buy := newOpenOrderForMatching(

		t,

		"buy-001",

		"BTCUSD",

		SideBuy,

		Price(6_000_000),

		Quantity(5_000_000),
	)

	sell := newOpenOrderForMatching(

		t,

		"sell-001",

		"BTCUSD",

		SideSell,

		Price(6_100_000),

		Quantity(5_000_000),
	)

	if err := book.Add(buy); err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if err := book.Add(sell); err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	trades, err := engine.Match(

		book,

		time.Date(

			2026,

			time.September,

			25,

			12,

			1,

			0,

			0,

			time.UTC,
		),
	)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(trades) != 0 {

		t.Fatalf(

			"expected 0 trades, got %d",

			len(trades),
		)

	}

	if buy.Status() != OrderStatusOpen {

		t.Errorf(

			"expected buy order OPEN, got %s",

			buy.Status(),
		)

	}

	if sell.Status() != OrderStatusOpen {

		t.Errorf(

			"expected sell order OPEN, got %s",

			sell.Status(),
		)

	}

}

func TestMatchingEngineCompletesEqualQuantityOrders(t *testing.T) {

	book := NewOrderBook()

	engine := NewMatchingEngine()

	sell := newOpenOrderForMatching(

		t,

		"sell-001",

		"BTCUSD",

		SideSell,

		Price(6_000_000),

		Quantity(5_000_000),
	)

	buy := newOpenOrderForMatching(

		t,

		"buy-001",

		"BTCUSD",

		SideBuy,

		Price(6_100_000),

		Quantity(5_000_000),
	)

	if err := book.Add(sell); err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if err := book.Add(buy); err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	trades, err := engine.Match(

		book,

		time.Date(

			2026,

			time.September,

			25,

			12,

			1,

			0,

			0,

			time.UTC,
		),
	)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(trades) != 1 {

		t.Fatalf(

			"expected 1 trade, got %d",

			len(trades),
		)

	}

	trade := trades[0]

	if trade.Quantity() != Quantity(5_000_000) {

		t.Errorf(

			"expected quantity %d, got %d",

			5_000_000,

			trade.Quantity(),
		)

	}

	if trade.Price() != Price(6_000_000) {

		t.Errorf(

			"expected resting ask price %d, got %d",

			6_000_000,

			trade.Price(),
		)

	}

	if buy.Status() != OrderStatusFilled {

		t.Errorf(

			"expected buy FILLED, got %s",

			buy.Status(),
		)

	}

	if sell.Status() != OrderStatusFilled {

		t.Errorf(

			"expected sell FILLED, got %s",

			sell.Status(),
		)

	}

	if len(book.Bids()) != 0 {

		t.Errorf(

			"expected 0 bids, got %d",

			len(book.Bids()),
		)

	}

	if len(book.Asks()) != 0 {

		t.Errorf(

			"expected 0 asks, got %d",

			len(book.Asks()),
		)

	}

}

func TestMatchingEngineUsesRestingBidPrice(t *testing.T) {
	book := NewOrderBook()
	engine := NewMatchingEngine()

	buy := newOpenOrderForMatching(
		t,
		"buy-001",
		"BTCUSD",
		SideBuy,
		Price(6_100_000),
		Quantity(5_000_000),
	)

	sell := newOpenOrderForMatching(
		t,
		"sell-001",
		"BTCUSD",
		SideSell,
		Price(6_000_000),
		Quantity(5_000_000),
	)

	if err := book.Add(buy); err != nil {
		t.Fatalf("unexpected error adding buy: %v", err)
	}

	if err := book.Add(sell); err != nil {
		t.Fatalf("unexpected error adding sell: %v", err)
	}

	trades, err := engine.Match(
		book,
		buy.UpdatedAt().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("matching orders: %v", err)
	}

	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	if trades[0].Price() != Price(6_100_000) {
		t.Errorf(
			"expected resting bid price %d, got %d",
			6_100_000,
			trades[0].Price(),
		)
	}
}

func TestMatchingEngineSupportsPartialFill(t *testing.T) {

	book := NewOrderBook()

	engine := NewMatchingEngine()

	sell := newOpenOrderForMatching(

		t,

		"sell-001",

		"BTCUSD",

		SideSell,

		Price(6_000_000),

		Quantity(2_000_000),
	)

	buy := newOpenOrderForMatching(

		t,

		"buy-001",

		"BTCUSD",

		SideBuy,

		Price(6_100_000),

		Quantity(5_000_000),
	)

	if err := book.Add(sell); err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if err := book.Add(buy); err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	trades, err := engine.Match(

		book,

		time.Date(

			2026,

			time.September,

			25,

			12,

			1,

			0,

			0,

			time.UTC,
		),
	)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(trades) != 1 {

		t.Fatalf(

			"expected 1 trade, got %d",

			len(trades),
		)

	}

	if trades[0].Quantity() != Quantity(2_000_000) {

		t.Errorf(

			"expected trade quantity %d, got %d",

			2_000_000,

			trades[0].Quantity(),
		)

	}

	if sell.Status() != OrderStatusFilled {

		t.Errorf(

			"expected sell FILLED, got %s",

			sell.Status(),
		)

	}

	if buy.Status() != OrderStatusPartiallyFilled {

		t.Errorf(

			"expected buy PARTIALLY_FILLED, got %s",

			buy.Status(),
		)

	}

	if buy.RemainingQuantity() != Quantity(3_000_000) {

		t.Errorf(

			"expected remaining quantity %d, got %d",

			3_000_000,

			buy.RemainingQuantity(),
		)

	}

	if len(book.Bids()) != 1 {

		t.Errorf(

			"expected 1 remaining bid, got %d",

			len(book.Bids()),
		)

	}

	if len(book.Asks()) != 0 {

		t.Errorf(

			"expected 0 asks, got %d",

			len(book.Asks()),
		)

	}

}

func TestMatchingEngineMatchesAgainstMultipleOrders(t *testing.T) {

	book := NewOrderBook()

	engine := NewMatchingEngine()

	sellOne := newOpenOrderForMatching(

		t,

		"sell-001",

		"BTCUSD",

		SideSell,

		Price(6_000_000),

		Quantity(2_000_000),
	)

	sellTwo := newOpenOrderForMatching(

		t,

		"sell-002",

		"BTCUSD",

		SideSell,

		Price(6_050_000),

		Quantity(3_000_000),
	)

	buy := newOpenOrderForMatching(

		t,

		"buy-001",

		"BTCUSD",

		SideBuy,

		Price(6_100_000),

		Quantity(5_000_000),
	)

	if err := book.Add(sellOne); err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if err := book.Add(sellTwo); err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if err := book.Add(buy); err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	trades, err := engine.Match(

		book,

		time.Date(

			2026,

			time.September,

			25,

			12,

			1,

			0,

			0,

			time.UTC,
		),
	)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if len(trades) != 2 {

		t.Fatalf(

			"expected 2 trades, got %d",

			len(trades),
		)

	}

	if trades[0].Quantity() != Quantity(2_000_000) {

		t.Errorf(

			"expected first trade quantity %d, got %d",

			2_000_000,

			trades[0].Quantity(),
		)

	}

	if trades[1].Quantity() != Quantity(3_000_000) {

		t.Errorf(

			"expected second trade quantity %d, got %d",

			3_000_000,

			trades[1].Quantity(),
		)

	}

	if trades[0].Price() != Price(6_000_000) {

		t.Errorf(

			"expected first trade price %d, got %d",

			6_000_000,

			trades[0].Price(),
		)

	}

	if trades[1].Price() != Price(6_050_000) {

		t.Errorf(

			"expected second trade price %d, got %d",

			6_050_000,

			trades[1].Price(),
		)

	}

	if buy.Status() != OrderStatusFilled {

		t.Errorf(

			"expected buy FILLED, got %s",

			buy.Status(),
		)

	}

	if sellOne.Status() != OrderStatusFilled {

		t.Errorf(

			"expected first sell FILLED, got %s",

			sellOne.Status(),
		)

	}

	if sellTwo.Status() != OrderStatusFilled {

		t.Errorf(

			"expected second sell FILLED, got %s",

			sellTwo.Status(),
		)

	}

}

func TestMatchingEngineRejectsNilOrderBook(t *testing.T) {
	engine := NewMatchingEngine()

	_, err := engine.Match(nil, time.Now().UTC())

	if !errors.Is(err, ErrMatchingNilOrderBook) {
		t.Fatalf(
			"expected %v, got %v",
			ErrMatchingNilOrderBook,
			err,
		)
	}
}

func TestMatchingEngineRejectsZeroTime(t *testing.T) {
	engine := NewMatchingEngine()

	_, err := engine.Match(
		NewOrderBook(),
		time.Time{},
	)

	if !errors.Is(err, ErrMatchingTimeRequired) {
		t.Fatalf(
			"expected %v, got %v",
			ErrMatchingTimeRequired,
			err,
		)
	}
}

func TestMatchingEngineDoesNotPartiallyMutateOnInvalidCounterOrder(

	t *testing.T,

) {

	book := NewOrderBook()

	engine := NewMatchingEngine()

	createdAt := time.Date(

		2026,

		time.September,

		25,

		12,

		0,

		0,

		0,

		time.UTC,
	)

	sell := newOpenOrderForMatching(

		t,

		"sell-001",

		"BTCUSD",

		SideSell,

		Price(6_000_000),

		Quantity(5_000_000),
	)

	buy := newOpenOrderForMatching(

		t,

		"buy-001",

		"BTCUSD",

		SideBuy,

		Price(6_100_000),

		Quantity(5_000_000),
	)

	if err := book.Add(sell); err != nil {

		t.Fatalf(

			"unexpected error adding sell: %v",

			err,
		)

	}

	if err := book.Add(buy); err != nil {

		t.Fatalf(

			"unexpected error adding buy: %v",

			err,
		)

	}

	cancelAt := createdAt.Add(2 * time.Minute)

	if err := sell.Cancel(cancelAt); err != nil {

		t.Fatalf(

			"failed to cancel sell order: %v",

			err,
		)

	}

	matchAt := createdAt.Add(3 * time.Minute)

	trades, err := engine.Match(

		book,

		matchAt,
	)

	if err == nil {

		t.Fatal("expected matching error")

	}

	if !errors.Is(err, ErrInvalidOrderTransition) {

		t.Fatalf(

			"expected %v, got %v",

			ErrInvalidOrderTransition,

			err,
		)

	}

	if len(trades) != 0 {

		t.Fatalf(

			"expected 0 trades, got %d",

			len(trades),
		)

	}

	if buy.Status() != OrderStatusOpen {

		t.Errorf(

			"expected buy status %s, got %s",

			OrderStatusOpen,

			buy.Status(),
		)

	}

	if buy.FilledQuantity() != 0 {

		t.Errorf(

			"expected buy filled quantity 0, got %d",

			buy.FilledQuantity(),
		)

	}

	if buy.RemainingQuantity() != Quantity(5_000_000) {

		t.Errorf(

			"expected buy remaining quantity %d, got %d",

			5_000_000,

			buy.RemainingQuantity(),
		)

	}

	if sell.Status() != OrderStatusCancelled {

		t.Errorf(

			"expected sell status %s, got %s",

			OrderStatusCancelled,

			sell.Status(),
		)

	}

	if sell.FilledQuantity() != 0 {

		t.Errorf(

			"expected sell filled quantity 0, got %d",

			sell.FilledQuantity(),
		)

	}

}
