package orders

import (
	"errors"
	"testing"
)

func TestParseSide(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Side
		wantErr error
	}{
		{
			name:  "buy",
			input: "BUY",
			want:  SideBuy,
		},
		{
			name:  "sell",
			input: "SELL",
			want:  SideSell,
		},
		{
			name:  "lowercase buy",
			input: "buy",
			want:  SideBuy,
		},
		{
			name:  "side with spaces",
			input: "  sell  ",
			want:  SideSell,
		},
		{
			name:    "invalid side",
			input:   "HOLD",
			wantErr: ErrInvalidSide,
		},
		{
			name:    "empty side",
			input:   "",
			wantErr: ErrInvalidSide,
		},
		{
			name:    "side with only spaces",
			input:   "   ",
			wantErr: ErrInvalidSide,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSide(tt.input)

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
				t.Errorf("expected side %q, got %q", tt.want, got)
			}
		})
	}
}
