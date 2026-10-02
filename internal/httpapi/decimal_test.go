package httpapi

import (
	"errors"
	"math"
	"testing"

	"github.com/Omar2709/pulseops/internal/orders"
)

func TestParseFixedPoint(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		scale   int64
		want    int64
		wantErr bool
	}{
		{
			name:  "price",
			value: "60000.25",
			scale: orders.PriceScale,
			want:  6_000_025,
		},
		{
			name:  "quantity",
			value: "0.05",
			scale: orders.QuantityScale,
			want:  5_000_000,
		},
		{
			name:  "integer price",
			value: "100",
			scale: orders.PriceScale,
			want:  10_000,
		},
		{
			name:  "zero",
			value: "0",
			scale: orders.PriceScale,
			want:  0,
		},
		{
			name:  "maximum int64",
			value: "92233720368547758.07",
			scale: orders.PriceScale,
			want:  math.MaxInt64,
		},
		{
			name:    "integer overflow",
			value:   "92233720368547758.08",
			scale:   orders.PriceScale,
			wantErr: true,
		},
		{
			name:  "maximum quantity",
			value: "92233720368.54775807",
			scale: orders.QuantityScale,
			want:  math.MaxInt64,
		},
		{
			name:    "quantity overflow",
			value:   "92233720368.54775808",
			scale:   orders.QuantityScale,
			wantErr: true,
		},
		{
			name:  "unit scale",
			value: "125",
			scale: 1,
			want:  125,
		},
		{
			name:    "invalid zero scale",
			value:   "10",
			scale:   0,
			wantErr: true,
		},
		{
			name:    "invalid negative scale",
			value:   "10",
			scale:   -100,
			wantErr: true,
		},
		{
			name:    "too many decimals",
			value:   "10.001",
			scale:   orders.PriceScale,
			wantErr: true,
		},
		{
			name:    "negative value",
			value:   "-1.00",
			scale:   orders.PriceScale,
			wantErr: true,
		},
		{
			name:    "positive sign",
			value:   "+1.00",
			scale:   orders.PriceScale,
			wantErr: true,
		},
		{
			name:    "empty value",
			value:   "",
			scale:   orders.PriceScale,
			wantErr: true,
		},
		{
			name:    "empty fraction",
			value:   "10.",
			scale:   orders.PriceScale,
			wantErr: true,
		},
		{
			name:    "scientific notation",
			value:   "1e3",
			scale:   orders.PriceScale,
			wantErr: true,
		},
		{
			name:    "invalid scale",
			value:   "10.50",
			scale:   12,
			wantErr: true,
		},
		{
			name:    "invalid fraction character",
			value:   "10.2x",
			scale:   orders.PriceScale,
			wantErr: true,
		},
		{
			name:  "leading zeros",
			value: "00010.5",
			scale: orders.PriceScale,
			want:  1050,
		},
		{
			name:  "surrounding whitespace",
			value: " 10.50 ",
			scale: orders.PriceScale,
			want:  1050,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFixedPoint(
				tt.value,
				tt.scale,
			)

			if tt.wantErr {
				if !errors.Is(err, ErrInvalidDecimal) {
					t.Fatalf(
						"expected %v, got %v",
						ErrInvalidDecimal,
						err,
					)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf(
					"expected %d, got %d",
					tt.want,
					got,
				)
			}
		})
	}
}
