package orders

import "errors"

type Price int64

const PriceScale int64 = 100

var ErrInvalidPrice = errors.New("price must be greater than zero")

func NewPrice(units int64) (Price, error) {
	if units <= 0 {
		return 0, ErrInvalidPrice
	}

	return Price(units), nil
}

func (p Price) Units() int64 {
	return int64(p)
}
