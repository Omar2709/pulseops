package orders

import "testing"

func TestOrderStatusIsTerminal(t *testing.T) {
	tests := []struct {
		name   string
		status OrderStatus
		want   bool
	}{
		{
			name:   "filled is terminal",
			status: OrderStatusFilled,
			want:   true,
		},
		{
			name:   "cancelled is terminal",
			status: OrderStatusCancelled,
			want:   true,
		},
		{
			name:   "rejected is terminal",
			status: OrderStatusRejected,
			want:   true,
		},
		{
			name:   "pending is not terminal",
			status: OrderStatusPending,
			want:   false,
		},
		{
			name:   "open is not terminal",
			status: OrderStatusOpen,
			want:   false,
		},
		{
			name:   "partially filled is not terminal",
			status: OrderStatusPartiallyFilled,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsTerminal()

			if got != tt.want {
				t.Errorf(
					"expected IsTerminal() to return %t, got %t",
					tt.want,
					got,
				)
			}
		})
	}
}

func TestOrderStatusCanTransitionTo(t *testing.T) {
	tests := []struct {
		name string
		from OrderStatus
		to   OrderStatus
		want bool
	}{
		{
			name: "pending to open",
			from: OrderStatusPending,
			to:   OrderStatusOpen,
			want: true,
		},
		{
			name: "pending to rejected",
			from: OrderStatusPending,
			to:   OrderStatusRejected,
			want: true,
		},
		{
			name: "open to partially filled",
			from: OrderStatusOpen,
			to:   OrderStatusPartiallyFilled,
			want: true,
		},
		{
			name: "open to filled",
			from: OrderStatusOpen,
			to:   OrderStatusFilled,
			want: true,
		},
		{
			name: "open to cancelled",
			from: OrderStatusOpen,
			to:   OrderStatusCancelled,
			want: true,
		},
		{
			name: "partially filled to filled",
			from: OrderStatusPartiallyFilled,
			to:   OrderStatusFilled,
			want: true,
		},
		{
			name: "partially filled to cancelled",
			from: OrderStatusPartiallyFilled,
			to:   OrderStatusCancelled,
			want: true,
		},

		// Invalid transitions.
		{
			name: "pending directly to filled",
			from: OrderStatusPending,
			to:   OrderStatusFilled,
			want: false,
		},
		{
			name: "filled to open",
			from: OrderStatusFilled,
			to:   OrderStatusOpen,
			want: false,
		},
		{
			name: "cancelled to open",
			from: OrderStatusCancelled,
			to:   OrderStatusOpen,
			want: false,
		},
		{
			name: "rejected to open",
			from: OrderStatusRejected,
			to:   OrderStatusOpen,
			want: false,
		},
		{
			name: "open to pending",
			from: OrderStatusOpen,
			to:   OrderStatusPending,
			want: false,
		},
		{
			name: "partially filled to open",
			from: OrderStatusPartiallyFilled,
			to:   OrderStatusOpen,
			want: false,
		},
		{
			name: "filled to cancelled",
			from: OrderStatusFilled,
			to:   OrderStatusCancelled,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.from.CanTransitionTo(tt.to)

			if got != tt.want {
				t.Errorf(
					"expected transition from %q to %q to be %t, got %t",
					tt.from,
					tt.to,
					tt.want,
					got,
				)
			}
		})
	}
}
