package httpapi

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var ErrInvalidDecimal = errors.New("invalid decimal value")

func parseFixedPoint(value string, scale int64) (int64, error) {
	if scale <= 0 {
		return 0, ErrInvalidDecimal
	}

	// The scale must be a power of ten.
	decimalPlaces := 0

	for n := scale; n > 1; n /= 10 {
		if n%10 != 0 {
			return 0, ErrInvalidDecimal
		}

		decimalPlaces++
	}

	value = strings.TrimSpace(value)

	wholePart, fractionPart, hasDecimal := strings.Cut(value, ".")

	if !isASCIIDigits(wholePart) {
		return 0, ErrInvalidDecimal
	}

	if hasDecimal {
		if !isASCIIDigits(fractionPart) {
			return 0, ErrInvalidDecimal
		}

		if len(fractionPart) > decimalPlaces {
			return 0, ErrInvalidDecimal
		}
	}

	whole, err := strconv.ParseInt(wholePart, 10, 64)
	if err != nil {
		return 0, ErrInvalidDecimal
	}

	var fraction int64

	if hasDecimal {
		padded := fractionPart + strings.Repeat(
			"0",
			decimalPlaces-len(fractionPart),
		)

		fraction, err = strconv.ParseInt(padded, 10, 64)
		if err != nil {
			return 0, ErrInvalidDecimal
		}
	}

	// Prevent overflow before multiplication and addition.
	if whole > (math.MaxInt64-fraction)/scale {
		return 0, ErrInvalidDecimal
	}

	return whole*scale + fraction, nil
}

func isASCIIDigits(value string) bool {
	if value == "" {
		return false
	}

	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}

	return true
}
