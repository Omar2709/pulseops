package orders

import (
	"errors"
	"testing"
)

func TestNewQuantity(t *testing.T) {
	tests := []struct {
		name    string
		units   int64
		want    Quantity
		wantErr error
	}{
		{
			name:  "valid quantity",
			units: 5_000_000,
			want:  Quantity(5_000_000),
		},
		{
			name:  "zero quantity",
			units: 0,
			want:  Quantity(0),
		},
		{
			name:    "negative quantity",
			units:   -1,
			wantErr: ErrInvalidQuantity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewQuantity(tt.units)

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
				t.Errorf("expected quantity %d, got %d", tt.want, got)
			}
		})
	}
}

func TestQuantityUnits(t *testing.T) {
	quantity := Quantity(5_000_000)

	got := quantity.Units()

	if got != 5_000_000 {
		t.Errorf("expected units %d, got %d", 5_000_000, got)
	}
}
