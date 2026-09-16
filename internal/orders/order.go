package orders

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrOrderIDRequired        = errors.New("order ID is required")
	ErrOrderSymbolRequired    = errors.New("order symbol is required")
	ErrOrderQuantityInvalid   = errors.New("order quantity must be greater than zero")
	ErrOrderCreatedAtRequired = errors.New("order creation time is required")
)

type Order struct {
	id             string
	symbol         string
	side           Side
	price          Price
	quantity       Quantity
	filledQuantity Quantity
	status         OrderStatus
	createdAt      time.Time
	updatedAt      time.Time
}

func NewOrder(
	id string,
	symbol string,
	side Side,
	price Price,
	quantity Quantity,
	createdAt time.Time,
) (*Order, error) {
	id = strings.TrimSpace(id)
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	if id == "" {
		return nil, ErrOrderIDRequired
	}

	if symbol == "" {
		return nil, ErrOrderSymbolRequired
	}

	if side != SideBuy && side != SideSell {
		return nil, ErrInvalidSide
	}

	if price <= 0 {
		return nil, ErrInvalidPrice
	}

	if quantity <= 0 {
		return nil, ErrOrderQuantityInvalid
	}

	if createdAt.IsZero() {
		return nil, ErrOrderCreatedAtRequired
	}

	order := &Order{
		id:             id,
		symbol:         symbol,
		side:           side,
		price:          price,
		quantity:       quantity,
		filledQuantity: 0,
		status:         OrderStatusPending,
		createdAt:      createdAt.UTC(),
		updatedAt:      createdAt.UTC(),
	}

	return order, nil
}

func (o *Order) ID() string {
	return o.id
}

func (o *Order) Symbol() string {
	return o.symbol
}

func (o *Order) Side() Side {
	return o.side
}

func (o *Order) Price() Price {
	return o.price
}

func (o *Order) Quantity() Quantity {
	return o.quantity
}

func (o *Order) FilledQuantity() Quantity {
	return o.filledQuantity
}

func (o *Order) Status() OrderStatus {
	return o.status
}

func (o *Order) CreatedAt() time.Time {
	return o.createdAt
}

func (o *Order) UpdatedAt() time.Time {
	return o.updatedAt
}
