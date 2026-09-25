package orders

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrTradeIDRequired          = errors.New("trade ID is required")
	ErrTradeSymbolRequired      = errors.New("trade symbol is required")
	ErrTradeBuyOrderIDRequired  = errors.New("buy order ID is required")
	ErrTradeSellOrderIDRequired = errors.New("sell order ID is required")
	ErrTradeOrdersMustDiffer    = errors.New("buy and sell order IDs must be different")
	ErrTradeQuantityInvalid     = errors.New("trade quantity must be greater than zero")
	ErrTradeExecutedAtRequired  = errors.New("trade execution time is required")
)

type Trade struct {
	id          string
	symbol      string
	buyOrderID  string
	sellOrderID string
	price       Price
	quantity    Quantity
	executedAt  time.Time
}

func NewTrade(
	id string,
	symbol string,
	buyOrderID string,
	sellOrderID string,
	price Price,
	quantity Quantity,
	executedAt time.Time,
) (*Trade, error) {
	id = strings.TrimSpace(id)
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	buyOrderID = strings.TrimSpace(buyOrderID)
	sellOrderID = strings.TrimSpace(sellOrderID)

	if id == "" {
		return nil, ErrTradeIDRequired
	}

	if symbol == "" {
		return nil, ErrTradeSymbolRequired
	}

	if buyOrderID == "" {
		return nil, ErrTradeBuyOrderIDRequired
	}

	if sellOrderID == "" {
		return nil, ErrTradeSellOrderIDRequired
	}

	if buyOrderID == sellOrderID {
		return nil, ErrTradeOrdersMustDiffer
	}

	if price <= 0 {
		return nil, ErrInvalidPrice
	}

	if quantity <= 0 {
		return nil, ErrTradeQuantityInvalid
	}

	if executedAt.IsZero() {
		return nil, ErrTradeExecutedAtRequired
	}

	return &Trade{
		id:          id,
		symbol:      symbol,
		buyOrderID:  buyOrderID,
		sellOrderID: sellOrderID,
		price:       price,
		quantity:    quantity,
		executedAt:  executedAt.UTC(),
	}, nil
}

func (t *Trade) ID() string {
	return t.id
}

func (t *Trade) Symbol() string {
	return t.symbol
}

func (t *Trade) BuyOrderID() string {
	return t.buyOrderID
}

func (t *Trade) SellOrderID() string {
	return t.sellOrderID
}

func (t *Trade) Price() Price {
	return t.price
}

func (t *Trade) Quantity() Quantity {
	return t.quantity
}

func (t *Trade) ExecutedAt() time.Time {
	return t.executedAt
}
