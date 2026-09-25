package orders

import (
	"errors"
	"testing"
	"time"
)

func newOpenOrderForBook(
	t *testing.T,
	id string,
	side Side,
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
		"BTCUSD",
		side,
		Price(6_000_000),
		Quantity(5_000_000),
		createdAt,
	)
	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}
	if err := order.Open(createdAt.Add(time.Second)); err != nil {
		t.Fatalf("failed to open order: %v", err)
	}
	return order
}

func TestOrderBookAddBuyOrder(t *testing.T) {
	book := NewOrderBook()
	order := newOpenOrderForBook(
		t,
		"buy-001",
		SideBuy,
	)
	if err := book.Add(order); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bids := book.Bids()
	asks := book.Asks()
	if len(bids) != 1 {
		t.Fatalf(
			"expected 1 bid, got %d",
			len(bids),
		)
	}
	if len(asks) != 0 {
		t.Fatalf(
			"expected 0 asks, got %d",
			len(asks),
		)
	}
	if bids[0].ID() != "buy-001" {
		t.Errorf(
			"expected order ID %q, got %q",
			"buy-001",
			bids[0].ID(),
		)
	}
}

func TestOrderBookAddSellOrder(t *testing.T) {
	book := NewOrderBook()
	order := newOpenOrderForBook(
		t,
		"sell-001",
		SideSell,
	)
	if err := book.Add(order); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(book.Bids()) != 0 {
		t.Fatalf(
			"expected 0 bids, got %d",
			len(book.Bids()),
		)
	}
	if len(book.Asks()) != 1 {
		t.Fatalf(
			"expected 1 ask, got %d",
			len(book.Asks()),
		)
	}
}

func TestOrderBookRejectsNilOrder(t *testing.T) {
	book := NewOrderBook()
	err := book.Add(nil)
	if !errors.Is(err, ErrOrderBookNilOrder) {
		t.Fatalf(
			"expected %v, got %v",
			ErrOrderBookNilOrder,
			err,
		)
	}
}

func TestOrderBookRejectsPendingOrder(t *testing.T) {
	book := NewOrderBook()
	createdAt := time.Now().UTC()
	order, err := NewOrder(
		"buy-001",
		"BTCUSD",
		SideBuy,
		Price(6_000_000),
		Quantity(5_000_000),
		createdAt,
	)
	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}
	err = book.Add(order)
	if !errors.Is(err, ErrOrderBookInactiveOrder) {
		t.Fatalf(
			"expected %v, got %v",
			ErrOrderBookInactiveOrder,
			err,
		)
	}
}

func TestOrderBookRejectsDuplicateOrder(t *testing.T) {
	book := NewOrderBook()
	order := newOpenOrderForBook(
		t,
		"buy-001",
		SideBuy,
	)
	if err := book.Add(order); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := book.Add(order)
	if !errors.Is(err, ErrOrderBookDuplicateOrder) {
		t.Fatalf(
			"expected %v, got %v",
			ErrOrderBookDuplicateOrder,
			err,
		)
	}
}

func TestOrderBookAcceptsPartiallyFilledOrder(t *testing.T) {
	book := NewOrderBook()
	order := newOpenOrderForBook(
		t,
		"buy-001",
		SideBuy,
	)
	fillTime := order.UpdatedAt().Add(time.Second)
	if err := order.ApplyFill(
		Quantity(2_000_000),
		fillTime,
	); err != nil {
		t.Fatalf(
			"failed to partially fill order: %v",
			err,
		)
	}
	if order.Status() != OrderStatusPartiallyFilled {
		t.Fatalf(
			"expected status %s, got %s",
			OrderStatusPartiallyFilled,
			order.Status(),
		)
	}
	if err := book.Add(order); err != nil {
		t.Fatalf(
			"unexpected error adding partially filled order: %v",
			err,
		)
	}
	if len(book.Bids()) != 1 {
		t.Fatalf(
			"expected 1 bid, got %d",
			len(book.Bids()),
		)
	}
}

func TestOrderBookRemoveBuyOrder(t *testing.T) {
	book := NewOrderBook()
	order := newOpenOrderForBook(
		t,
		"buy-001",
		SideBuy,
	)
	if err := book.Add(order); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Remove("buy-001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(book.Bids()) != 0 {
		t.Fatalf(
			"expected 0 bids, got %d",
			len(book.Bids()),
		)
	}
}

func TestOrderBookRemoveSellOrder(t *testing.T) {
	book := NewOrderBook()
	order := newOpenOrderForBook(
		t,
		"sell-001",
		SideSell,
	)
	if err := book.Add(order); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Remove("sell-001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(book.Asks()) != 0 {
		t.Fatalf(
			"expected 0 asks, got %d",
			len(book.Asks()),
		)
	}
}

func TestOrderBookCancelRemovesOrder(t *testing.T) {
	book := NewOrderBook()
	order := newOpenOrderForBook(
		t,
		"sell-001",
		SideSell,
	)
	if err := book.Add(order); err != nil {
		t.Fatalf("unexpected error adding order: %v", err)
	}
	cancelAt := order.UpdatedAt().Add(time.Second)
	if err := book.Cancel("sell-001", cancelAt); err != nil {
		t.Fatalf("unexpected error cancelling order: %v", err)
	}
	if order.Status() != OrderStatusCancelled {
		t.Fatalf(
			"expected status %s, got %s",
			OrderStatusCancelled,
			order.Status(),
		)
	}
	if len(book.Asks()) != 0 {
		t.Fatalf(
			"expected 0 asks after cancellation, got %d",
			len(book.Asks()),
		)
	}
}

func TestOrderBookRemoveUnknownOrder(t *testing.T) {
	book := NewOrderBook()
	err := book.Remove("unknown-order")
	if !errors.Is(err, ErrOrderBookOrderNotFound) {
		t.Fatalf(
			"expected %v, got %v",
			ErrOrderBookOrderNotFound,
			err,
		)
	}
}

func newOpenOrderForBookWithPrice(
	t *testing.T,
	id string,
	side Side,
	price Price,
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
		"BTCUSD",
		side,
		price,
		Quantity(5_000_000),
		createdAt,
	)
	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}
	if err := order.Open(
		createdAt.Add(time.Second),
	); err != nil {
		t.Fatalf("failed to open order: %v", err)
	}
	return order
}

func TestOrderBookBidsUsePricePriority(t *testing.T) {
	book := NewOrderBook()
	low := newOpenOrderForBookWithPrice(
		t,
		"buy-low",
		SideBuy,
		Price(6_000_000),
	)
	high := newOpenOrderForBookWithPrice(
		t,
		"buy-high",
		SideBuy,
		Price(6_100_000),
	)
	middle := newOpenOrderForBookWithPrice(
		t,
		"buy-middle",
		SideBuy,
		Price(6_050_000),
	)
	if err := book.Add(low); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Add(high); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Add(middle); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bids := book.Bids()
	if len(bids) != 3 {
		t.Fatalf(
			"expected 3 bids, got %d",
			len(bids),
		)
	}
	expected := []string{
		"buy-high",
		"buy-middle",
		"buy-low",
	}
	for i, expectedID := range expected {
		if bids[i].ID() != expectedID {
			t.Errorf(
				"position %d: expected %s, got %s",
				i,
				expectedID,
				bids[i].ID(),
			)
		}
	}
}

func TestOrderBookAsksUsePricePriority(t *testing.T) {
	book := NewOrderBook()
	high := newOpenOrderForBookWithPrice(
		t,
		"sell-high",
		SideSell,
		Price(6_100_000),
	)
	low := newOpenOrderForBookWithPrice(
		t,
		"sell-low",
		SideSell,
		Price(6_000_000),
	)
	middle := newOpenOrderForBookWithPrice(
		t,
		"sell-middle",
		SideSell,
		Price(6_050_000),
	)
	if err := book.Add(high); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Add(low); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Add(middle); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	asks := book.Asks()
	if len(asks) != 3 {
		t.Fatalf(
			"expected 3 asks, got %d",
			len(asks),
		)
	}
	expected := []string{
		"sell-low",
		"sell-middle",
		"sell-high",
	}
	for i, expectedID := range expected {
		if asks[i].ID() != expectedID {
			t.Errorf(
				"position %d: expected %s, got %s",
				i,
				expectedID,
				asks[i].ID(),
			)
		}
	}
}

func TestOrderBookBidsUseTimePriorityWhenPricesMatch(t *testing.T) {
	book := NewOrderBook()
	first := newOpenOrderForBookWithPrice(
		t,
		"buy-first",
		SideBuy,
		Price(6_000_000),
	)
	second := newOpenOrderForBookWithPrice(
		t,
		"buy-second",
		SideBuy,
		Price(6_000_000),
	)
	third := newOpenOrderForBookWithPrice(
		t,
		"buy-third",
		SideBuy,
		Price(6_000_000),
	)
	if err := book.Add(first); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Add(second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Add(third); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bids := book.Bids()
	expected := []string{
		"buy-first",
		"buy-second",
		"buy-third",
	}
	for i, expectedID := range expected {
		if bids[i].ID() != expectedID {
			t.Errorf(
				"position %d: expected %s, got %s",
				i,
				expectedID,
				bids[i].ID(),
			)
		}
	}
}

func TestOrderBookAsksUseTimePriorityWhenPricesMatch(t *testing.T) {
	book := NewOrderBook()
	first := newOpenOrderForBookWithPrice(
		t,
		"sell-first",
		SideSell,
		Price(6_000_000),
	)
	second := newOpenOrderForBookWithPrice(
		t,
		"sell-second",
		SideSell,
		Price(6_000_000),
	)
	third := newOpenOrderForBookWithPrice(
		t,
		"sell-third",
		SideSell,
		Price(6_000_000),
	)
	if err := book.Add(first); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Add(second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := book.Add(third); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	asks := book.Asks()
	expected := []string{
		"sell-first",
		"sell-second",
		"sell-third",
	}
	for i, expectedID := range expected {
		if asks[i].ID() != expectedID {
			t.Errorf(
				"position %d: expected %s, got %s",
				i,
				expectedID,
				asks[i].ID(),
			)
		}
	}
}

func TestOrderBookRejectsDifferentSymbol(t *testing.T) {
	book := NewOrderBook()
	btc := newOpenOrderForMatching(
		t,
		"buy-btc",
		"BTCUSD",
		SideBuy,
		Price(6_000_000),
		Quantity(5_000_000),
	)
	eth := newOpenOrderForMatching(
		t,
		"sell-eth",
		"ETHUSD",
		SideSell,
		Price(300_000),
		Quantity(5_000_000),
	)
	if err := book.Add(btc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := book.Add(eth)
	if !errors.Is(err, ErrOrderBookSymbolMismatch) {
		t.Fatalf(
			"expected %v, got %v",
			ErrOrderBookSymbolMismatch,
			err,
		)
	}
}
