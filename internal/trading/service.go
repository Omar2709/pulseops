package trading

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

var (
	ErrDuplicateOrder      = errors.New("order already exists")
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderNotCancellable = errors.New("order cannot be cancelled")
)

type SubmitOrderInput struct {
	ID       string
	Symbol   string
	Side     orders.Side
	Price    orders.Price
	Quantity orders.Quantity
}

type OrderSnapshot struct {
	ID                string
	Symbol            string
	Side              orders.Side
	Price             orders.Price
	Quantity          orders.Quantity
	FilledQuantity    orders.Quantity
	RemainingQuantity orders.Quantity
	Status            orders.OrderStatus
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func snapshotOrder(order *orders.Order) OrderSnapshot {
	return OrderSnapshot{
		ID:                order.ID(),
		Symbol:            order.Symbol(),
		Side:              order.Side(),
		Price:             order.Price(),
		Quantity:          order.Quantity(),
		FilledQuantity:    order.FilledQuantity(),
		RemainingQuantity: order.RemainingQuantity(),
		Status:            order.Status(),
		CreatedAt:         order.CreatedAt(),
		UpdatedAt:         order.UpdatedAt(),
	}
}

type SubmitOrderResult struct {
	Order    OrderSnapshot
	Trades   []*orders.Trade
	Replayed bool
}

type Service struct {
	// mu protects the registry, books, global trade sequence, and commits.
	mu     sync.RWMutex
	books  map[string]*orders.OrderBook
	orders map[string]*orders.Order
	engine *orders.MatchingEngine
	store  StateStore
	now    func() time.Time

	// Symbol-specific gates permit matching work on different books to
	// proceed concurrently while retaining FIFO for each symbol.
	symbolLocks sync.Map // map[string]*sync.Mutex
}

func NewService() *Service {
	return newService(func() time.Time {
		return time.Now().UTC()
	})
}

func NewServiceWithStore(store StateStore) *Service {
	return newServiceWithStore(
		func() time.Time {
			return time.Now().UTC()
		},
		store,
	)
}

func newService(now func() time.Time) *Service {
	return newServiceWithStore(
		now,
		noopStateStore{},
	)
}

func newServiceWithStore(
	now func() time.Time,
	store StateStore,
) *Service {
	if store == nil {
		panic("state store cannot be nil")
	}

	return &Service{
		books:  make(map[string]*orders.OrderBook),
		orders: make(map[string]*orders.Order),
		engine: orders.NewMatchingEngine(),
		store:  store,
		now:    now,
	}
}

// symbolLock returns the stable gate for one normalized symbol. Locks are
// retained for the service lifetime; they must never be replaced while in use.
func (s *Service) symbolLock(symbol string) *sync.Mutex {
	lock, _ := s.symbolLocks.LoadOrStore(symbol, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func replayOrder(existing, incoming *orders.Order) (SubmitOrderResult, error) {
	if existing.Symbol() != incoming.Symbol() ||
		existing.Side() != incoming.Side() ||
		existing.Price() != incoming.Price() ||
		existing.Quantity() != incoming.Quantity() {
		return SubmitOrderResult{}, ErrDuplicateOrder
	}

	return SubmitOrderResult{
		Order:    snapshotOrder(existing),
		Replayed: true,
	}, nil
}

// SubmitOrder builds a private working book under a per-symbol gate, then
// assigns global trade IDs and persists under the service's commit lock.
// Different symbols can match concurrently without duplicating trade IDs.
func (s *Service) SubmitOrder(
	ctx context.Context,
	input SubmitOrderInput,
) (SubmitOrderResult, error) {
	// Normalize the symbol to select its stable gate. The same symbol
	// cannot race against cancellation or another submission.
	symbol := strings.ToUpper(strings.TrimSpace(input.Symbol))
	gate := s.symbolLock(symbol)
	gate.Lock()
	defer gate.Unlock()

	// Work inside the symbol gate, but outside the global commit lock.
	at := s.now()
	order, err := orders.NewOrder(
		input.ID,
		input.Symbol,
		input.Side,
		input.Price,
		input.Quantity,
		at,
	)
	if err != nil {
		return SubmitOrderResult{}, err
	}

	// The gate excludes all mutations of this symbol, including cancels.
	// Different symbols can change the registry while this book is staged.
	s.mu.RLock()
	if existing, exists := s.orders[order.ID()]; exists {
		result, err := replayOrder(existing, order)
		s.mu.RUnlock()
		return result, err
	}
	originalBook := s.books[order.Symbol()]
	s.mu.RUnlock()

	if err := order.Open(at); err != nil {
		return SubmitOrderResult{}, err
	}

	var workingBook *orders.OrderBook
	var workingOrders map[string]*orders.Order
	if originalBook != nil {
		// Safe without mu: the symbol gate excludes mutations of this book.
		workingBook, workingOrders = originalBook.Clone()
	} else {
		workingBook = orders.NewOrderBook()
		workingOrders = make(map[string]*orders.Order)
	}
	if err := workingBook.Add(order); err != nil {
		return SubmitOrderResult{}, err
	}

	// This engine is provisional: its temporary IDs are never published.
	// Global IDs are allocated only after matching succeeds at commit time.
	provisional, err := orders.NewMatchingEngine().Match(workingBook, at)
	if err != nil {
		return SubmitOrderResult{}, err
	}

	changedOrders := make([]OrderSnapshot, 0, len(workingOrders)+1)
	for _, updatedOrder := range workingOrders {
		changedOrders = append(changedOrders, snapshotOrder(updatedOrder))
	}
	changedOrders = append(changedOrders, snapshotOrder(order))

	// Commit is globally serialized because trade IDs and durable sequence
	// are global. The global gate also protects duplicate IDs across symbols.
	s.mu.Lock()
	defer s.mu.Unlock()

	// An order with this ID may have committed on another symbol while
	// the private working book was matching. Discard provisional work.
	if existing, exists := s.orders[order.ID()]; exists {
		return replayOrder(existing, order)
	}

	sequence := s.engine.TradeSequence()
	if uint64(len(provisional)) > ^uint64(0)-sequence {
		return SubmitOrderResult{}, fmt.Errorf("trade sequence exhausted")
	}

	trades := make([]*orders.Trade, 0, len(provisional))
	for _, tentative := range provisional {
		sequence++
		confirmed, err := orders.NewTrade(
			fmt.Sprintf("trade-%06d", sequence),
			tentative.Symbol(),
			tentative.BuyOrderID(),
			tentative.SellOrderID(),
			tentative.Price(),
			tentative.Quantity(),
			tentative.ExecutedAt(),
		)
		if err != nil {
			return SubmitOrderResult{}, fmt.Errorf(
				"finalize trade %s: %w", tentative.ID(), err,
			)
		}
		trades = append(trades, confirmed)
	}
	confirmedEngine := orders.NewMatchingEngineWithSequence(sequence)

	if err := s.store.Apply(
		ctx,
		StateChange{
			Orders:        changedOrders,
			Trades:        trades,
			BookSequences: workingBook.Sequences(),
			TradeSequence: confirmedEngine.TradeSequence(),
		},
	); err != nil {
		return SubmitOrderResult{}, fmt.Errorf(
			"persist order submission: %w",
			err,
		)
	}

	// Publish only after durable success. Cloned orders that were filled
	// and removed from the book are still updated in the registry.
	for id, updatedOrder := range workingOrders {
		s.orders[id] = updatedOrder
	}
	s.orders[order.ID()] = order
	s.books[order.Symbol()] = workingBook
	s.engine = confirmedEngine

	return SubmitOrderResult{
		Order:  snapshotOrder(order),
		Trades: trades,
	}, nil
}

// GetOrder returns a snapshot of the current state of an order.
// The registry retains filled orders even after they leave the book.
func (s *Service) GetOrder(
	id string,
) (OrderSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id = strings.TrimSpace(id)

	order, exists := s.orders[id]
	if !exists {
		return OrderSnapshot{}, ErrOrderNotFound
	}

	return snapshotOrder(order), nil
}

// CancelOrder removes an active order from its book but keeps it in the
// registry so it remains available through GetOrder.
func (s *Service) CancelOrder(
	ctx context.Context,
	id string,
) (OrderSnapshot, error) {
	id = strings.TrimSpace(id)

	// Read the immutable symbol before acquiring its gate; otherwise
	// cancellation could race with matching on a private working book.
	s.mu.RLock()
	order, exists := s.orders[id]
	if !exists {
		s.mu.RUnlock()
		return OrderSnapshot{}, ErrOrderNotFound
	}
	symbol := order.Symbol()
	s.mu.RUnlock()

	gate := s.symbolLock(symbol)
	gate.Lock()
	defer gate.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	order, exists = s.orders[id]
	if !exists {
		return OrderSnapshot{}, ErrOrderNotFound
	}

	if order.Status() != orders.OrderStatusOpen &&
		order.Status() != orders.OrderStatusPartiallyFilled {
		return OrderSnapshot{}, ErrOrderNotCancellable
	}

	book, exists := s.books[order.Symbol()]
	if !exists {
		return OrderSnapshot{}, fmt.Errorf(
			"missing order book for symbol %s",
			order.Symbol(),
		)
	}

	workingBook, workingOrders := book.Clone()

	workingOrder, exists := workingOrders[order.ID()]
	if !exists {
		return OrderSnapshot{}, fmt.Errorf(
			"missing cloned order %s",
			order.ID(),
		)
	}

	if err := workingBook.Cancel(
		workingOrder.ID(),
		s.now(),
	); err != nil {
		return OrderSnapshot{}, fmt.Errorf(
			"cancel order %s: %w",
			workingOrder.ID(),
			err,
		)
	}

	cancelled := snapshotOrder(workingOrder)

	if err := s.store.Apply(
		ctx,
		StateChange{
			Orders:        []OrderSnapshot{cancelled},
			TradeSequence: s.engine.TradeSequence(),
		},
	); err != nil {
		return OrderSnapshot{}, fmt.Errorf(
			"persist order cancellation: %w",
			err,
		)
	}

	s.books[workingOrder.Symbol()] = workingBook
	s.orders[workingOrder.ID()] = workingOrder

	return cancelled, nil
}
