package orders

import (
	"errors"
	"strings"
)

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

var ErrInvalidSide = errors.New("invalid order side")

func ParseSide(value string) (Side, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))

	switch normalized {
	case string(SideBuy):
		return SideBuy, nil
	case string(SideSell):
		return SideSell, nil
	default:
		return "", ErrInvalidSide
	}
}
