package trading

import (
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
	Order  OrderSnapshot
	Trades []*orders.Trade
}

type Service struct {
	mu     sync.RWMutex
	books  map[string]*orders.OrderBook
	orders map[string]*orders.Order
	engine *orders.MatchingEngine
	now    func() time.Time
}

func NewService() *Service {
	return newService(func() time.Time {
		return time.Now().UTC()
	})
}

func newService(now func() time.Time) *Service {
	return &Service{
		books:  make(map[string]*orders.OrderBook),
		orders: make(map[string]*orders.Order),
		engine: orders.NewMatchingEngine(),
		now:    now,
	}
}

// bookFor must be called while holding s.mu.
func (s *Service) bookFor(symbol string) *orders.OrderBook {
	book, exists := s.books[symbol]
	if exists {
		return book
	}

	book = orders.NewOrderBook()
	s.books[symbol] = book

	return book
}

func (s *Service) SubmitOrder(
	input SubmitOrderInput,
) (SubmitOrderResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	if _, exists := s.orders[order.ID()]; exists {
		return SubmitOrderResult{}, ErrDuplicateOrder
	}

	if err := order.Open(at); err != nil {
		return SubmitOrderResult{}, err
	}

	book := s.bookFor(order.Symbol())

	if err := book.Add(order); err != nil {
		return SubmitOrderResult{}, err
	}

	s.orders[order.ID()] = order

	trades, err := s.engine.Match(book, at)
	if err != nil {
		return SubmitOrderResult{}, err
	}

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
	id string,
) (OrderSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)

	order, exists := s.orders[id]
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

	if err := book.Cancel(order.ID(), s.now()); err != nil {
		return OrderSnapshot{}, fmt.Errorf(
			"cancel order %s: %w",
			order.ID(),
			err,
		)
	}

	return snapshotOrder(order), nil
}
