package orders

import (
	"errors"
	"testing"
)

func TestNewPrice(t *testing.T) {
	tests := []struct {
		name    string
		units   int64
		want    Price
		wantErr error
	}{
		{
			name:  "valid price",
			units: 6_000_025,
			want:  Price(6_000_025),
		},
		{
			name:    "zero price",
			units:   0,
			wantErr: ErrInvalidPrice,
		},
		{
			name:    "negative price",
			units:   -1,
			wantErr: ErrInvalidPrice,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPrice(tt.units)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if got != tt.want {
				t.Errorf("expected price %d, got %d", tt.want, got)
			}
		})
	}
}

func TestPriceUnits(t *testing.T) {
	price := Price(6_000_025)

	got := price.Units()

	if got != 6_000_025 {
		t.Errorf("expected units %d, got %d", 6_000_025, got)
	}
}
