package orders

import "errors"

type Quantity int64

const QuantityScale int64 = 100_000_000

var ErrInvalidQuantity = errors.New("quantity must not be negative")

func NewQuantity(units int64) (Quantity, error) {
	if units < 0 {
		return 0, ErrInvalidQuantity
	}

	return Quantity(units), nil
}

func (q Quantity) Units() int64 {
	return int64(q)
}
