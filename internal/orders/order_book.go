package orders

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrOrderBookNilOrder       = errors.New("order cannot be nil")
	ErrOrderBookDuplicateOrder = errors.New("order already exists in order book")
	ErrOrderBookInactiveOrder  = errors.New("order must be open or partially filled")
	ErrOrderBookOrderNotFound  = errors.New("order not found in order book")
	ErrOrderBookSymbolMismatch = errors.New("order symbol does not match order book")
)

type orderBookEntry struct {
	order    *Order
	sequence uint64
}

type OrderBook struct {
	symbol       string
	bids         map[string]orderBookEntry
	asks         map[string]orderBookEntry
	nextSequence uint64
}

func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids: make(map[string]orderBookEntry),
		asks: make(map[string]orderBookEntry),
	}
}

func (b *OrderBook) Add(order *Order) error {
	if order == nil {
		return ErrOrderBookNilOrder
	}

	if order.Status() != OrderStatusOpen &&
		order.Status() != OrderStatusPartiallyFilled {
		return fmt.Errorf(
			"%w: order %s has status %s",
			ErrOrderBookInactiveOrder,
			order.ID(),
			order.Status(),
		)
	}

	if b.contains(order.ID()) {
		return fmt.Errorf(
			"%w: %s",
			ErrOrderBookDuplicateOrder,
			order.ID(),
		)
	}

	if b.symbol != "" && order.Symbol() != b.symbol {
		return fmt.Errorf(
			"%w: expected %s, got %s",
			ErrOrderBookSymbolMismatch,
			b.symbol,
			order.Symbol(),
		)
	}

	entry := orderBookEntry{
		order:    order,
		sequence: b.nextSequence,
	}

	switch order.Side() {
	case SideBuy:
		b.bids[order.ID()] = entry

	case SideSell:
		b.asks[order.ID()] = entry

	default:
		return ErrInvalidSide
	}

	if b.symbol == "" {
		b.symbol = order.Symbol()
	}

	b.nextSequence++

	return nil
}

func (b *OrderBook) Remove(orderID string) error {
	orderID = strings.TrimSpace(orderID)

	if _, exists := b.bids[orderID]; exists {
		delete(b.bids, orderID)
		return nil
	}

	if _, exists := b.asks[orderID]; exists {
		delete(b.asks, orderID)
		return nil
	}

	return fmt.Errorf(
		"%w: %s",
		ErrOrderBookOrderNotFound,
		orderID,
	)
}

func (b *OrderBook) Cancel(orderID string, at time.Time) error {
	orderID = strings.TrimSpace(orderID)

	if entry, exists := b.bids[orderID]; exists {
		if err := entry.order.Cancel(at); err != nil {
			return err
		}

		delete(b.bids, orderID)
		return nil
	}

	if entry, exists := b.asks[orderID]; exists {
		if err := entry.order.Cancel(at); err != nil {
			return err
		}

		delete(b.asks, orderID)
		return nil
	}

	return fmt.Errorf(
		"%w: %s",
		ErrOrderBookOrderNotFound,
		orderID,
	)
}

func (b *OrderBook) Bids() []*Order {
	entries := make([]orderBookEntry, 0, len(b.bids))

	for _, entry := range b.bids {
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]

		if left.order.Price() == right.order.Price() {
			return left.sequence < right.sequence
		}

		return left.order.Price() > right.order.Price()
	})

	orders := make([]*Order, 0, len(entries))

	for _, entry := range entries {
		orders = append(orders, entry.order)
	}

	return orders
}

func (b *OrderBook) Asks() []*Order {
	entries := make([]orderBookEntry, 0, len(b.asks))

	for _, entry := range b.asks {
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]

		if left.order.Price() == right.order.Price() {
			return left.sequence < right.sequence
		}

		return left.order.Price() < right.order.Price()
	})

	orders := make([]*Order, 0, len(entries))

	for _, entry := range entries {
		orders = append(orders, entry.order)
	}

	return orders
}

func (b *OrderBook) contains(orderID string) bool {
	if _, exists := b.bids[orderID]; exists {
		return true
	}

	if _, exists := b.asks[orderID]; exists {
		return true
	}

	return false
}
