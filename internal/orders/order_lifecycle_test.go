package orders

import (
	"errors"
	"testing"
	"time"
)

func mustNewTestOrder(t *testing.T) *Order {
	t.Helper()

	createdAt := time.Date(
		2026,
		time.September,
		16,
		17,
		0,
		0,
		0,
		time.UTC,
	)

	order, err := NewOrder(
		"ord_001",
		"BTC-USD",
		SideBuy,
		Price(6_000_025),
		Quantity(5_000_000),
		createdAt,
	)

	if err != nil {
		t.Fatalf("creating test order: %v", err)
	}

	return order
}

func TestOrderOpen(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	err := order.Open(openedAt)
	if err != nil {
		t.Fatalf("opening order: %v", err)
	}

	if order.Status() != OrderStatusOpen {
		t.Errorf(
			"expected status %q, got %q",
			OrderStatusOpen,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(openedAt) {
		t.Errorf(
			"expected updated at %v, got %v",
			openedAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderReject(t *testing.T) {
	order := mustNewTestOrder(t)

	rejectedAt := order.CreatedAt().Add(time.Second)

	err := order.Reject(rejectedAt)
	if err != nil {
		t.Fatalf("rejecting order: %v", err)
	}

	if order.Status() != OrderStatusRejected {
		t.Errorf(
			"expected status %q, got %q",
			OrderStatusRejected,
			order.Status(),
		)
	}
}

func TestOrderCancel(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	cancelledAt := openedAt.Add(time.Second)

	if err := order.Cancel(cancelledAt); err != nil {
		t.Fatalf("cancelling order: %v", err)
	}

	if order.Status() != OrderStatusCancelled {
		t.Errorf(
			"expected status %q, got %q",
			OrderStatusCancelled,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(cancelledAt) {
		t.Errorf(
			"expected updated at %v, got %v",
			cancelledAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderCancelFromPendingFails(t *testing.T) {
	order := mustNewTestOrder(t)

	originalUpdatedAt := order.UpdatedAt()

	err := order.Cancel(order.CreatedAt().Add(time.Second))

	if !errors.Is(err, ErrInvalidOrderTransition) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrInvalidOrderTransition,
			err,
		)
	}

	if order.Status() != OrderStatusPending {
		t.Errorf(
			"expected status to remain %q, got %q",
			OrderStatusPending,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(originalUpdatedAt) {
		t.Errorf(
			"expected updated at to remain %v, got %v",
			originalUpdatedAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderOpenWithZeroTimeFails(t *testing.T) {
	order := mustNewTestOrder(t)

	err := order.Open(time.Time{})

	if !errors.Is(err, ErrOrderUpdateTimeRequired) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrOrderUpdateTimeRequired,
			err,
		)
	}

	if order.Status() != OrderStatusPending {
		t.Errorf(
			"expected status to remain %q, got %q",
			OrderStatusPending,
			order.Status(),
		)
	}
}

func TestOrderOpenWithOlderTimeFails(t *testing.T) {
	order := mustNewTestOrder(t)

	olderTime := order.UpdatedAt().Add(-time.Second)

	err := order.Open(olderTime)

	if !errors.Is(err, ErrOrderUpdateTimeBeforeCurrent) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrOrderUpdateTimeBeforeCurrent,
			err,
		)
	}

	if order.Status() != OrderStatusPending {
		t.Errorf(
			"expected status to remain %q, got %q",
			OrderStatusPending,
			order.Status(),
		)
	}
}

func TestOrderCannotOpenTwice(t *testing.T) {
	order := mustNewTestOrder(t)

	firstOpenedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(firstOpenedAt); err != nil {
		t.Fatalf("opening order first time: %v", err)
	}

	originalUpdatedAt := order.UpdatedAt()

	secondOpenedAt := firstOpenedAt.Add(time.Second)

	err := order.Open(secondOpenedAt)

	if !errors.Is(err, ErrInvalidOrderTransition) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrInvalidOrderTransition,
			err,
		)
	}

	if order.Status() != OrderStatusOpen {
		t.Errorf(
			"expected status to remain %q, got %q",
			OrderStatusOpen,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(originalUpdatedAt) {
		t.Errorf(
			"expected updated at to remain %v, got %v",
			originalUpdatedAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderRemainingQuantity(t *testing.T) {
	order := mustNewTestOrder(t)

	if order.RemainingQuantity() != order.Quantity() {
		t.Errorf(
			"expected remaining quantity %d, got %d",
			order.Quantity(),
			order.RemainingQuantity(),
		)
	}
}

func TestOrderApplyPartialFill(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	fill := Quantity(2_000_000)
	filledAt := openedAt.Add(time.Second)

	if err := order.ApplyFill(fill, filledAt); err != nil {
		t.Fatalf("applying fill: %v", err)
	}

	if order.FilledQuantity() != fill {
		t.Errorf(
			"expected filled quantity %d, got %d",
			fill,
			order.FilledQuantity(),
		)
	}

	expectedRemaining := Quantity(3_000_000)

	if order.RemainingQuantity() != expectedRemaining {
		t.Errorf(
			"expected remaining quantity %d, got %d",
			expectedRemaining,
			order.RemainingQuantity(),
		)
	}

	if order.Status() != OrderStatusPartiallyFilled {
		t.Errorf(
			"expected status %q, got %q",
			OrderStatusPartiallyFilled,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(filledAt) {
		t.Errorf(
			"expected updated at %v, got %v",
			filledAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderApplyFullFill(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	filledAt := openedAt.Add(time.Second)

	if err := order.ApplyFill(order.Quantity(), filledAt); err != nil {
		t.Fatalf("applying full fill: %v", err)
	}

	if order.FilledQuantity() != order.Quantity() {
		t.Errorf(
			"expected filled quantity %d, got %d",
			order.Quantity(),
			order.FilledQuantity(),
		)
	}

	if order.RemainingQuantity() != 0 {
		t.Errorf(
			"expected remaining quantity %d, got %d",
			0,
			order.RemainingQuantity(),
		)
	}

	if order.Status() != OrderStatusFilled {
		t.Errorf(
			"expected status %q, got %q",
			OrderStatusFilled,
			order.Status(),
		)
	}
}

func TestOrderApplyFillCompletesPartiallyFilledOrder(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	firstFillAt := openedAt.Add(time.Second)

	if err := order.ApplyFill(Quantity(2_000_000), firstFillAt); err != nil {
		t.Fatalf("applying first fill: %v", err)
	}

	secondFillAt := firstFillAt.Add(time.Second)

	if err := order.ApplyFill(Quantity(3_000_000), secondFillAt); err != nil {
		t.Fatalf("applying second fill: %v", err)
	}

	if order.FilledQuantity() != order.Quantity() {
		t.Errorf(
			"expected filled quantity %d, got %d",
			order.Quantity(),
			order.FilledQuantity(),
		)
	}

	if order.RemainingQuantity() != 0 {
		t.Errorf(
			"expected remaining quantity %d, got %d",
			0,
			order.RemainingQuantity(),
		)
	}

	if order.Status() != OrderStatusFilled {
		t.Errorf(
			"expected status %q, got %q",
			OrderStatusFilled,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(secondFillAt) {
		t.Errorf(
			"expected updated at %v, got %v",
			secondFillAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderApplyFillExceedingRemainingFails(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	originalUpdatedAt := order.UpdatedAt()

	err := order.ApplyFill(
		Quantity(6_000_000),
		openedAt.Add(time.Second),
	)

	if !errors.Is(err, ErrFillExceedsRemainingQuantity) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrFillExceedsRemainingQuantity,
			err,
		)
	}

	if order.FilledQuantity() != 0 {
		t.Errorf(
			"expected filled quantity to remain %d, got %d",
			0,
			order.FilledQuantity(),
		)
	}

	if order.Status() != OrderStatusOpen {
		t.Errorf(
			"expected status to remain %q, got %q",
			OrderStatusOpen,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(originalUpdatedAt) {
		t.Errorf(
			"expected updated at to remain %v, got %v",
			originalUpdatedAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderApplyFillWhilePendingFails(t *testing.T) {
	order := mustNewTestOrder(t)

	err := order.ApplyFill(
		Quantity(1_000_000),
		order.CreatedAt().Add(time.Second),
	)

	if !errors.Is(err, ErrInvalidOrderTransition) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrInvalidOrderTransition,
			err,
		)
	}

	if order.FilledQuantity() != 0 {
		t.Errorf(
			"expected filled quantity to remain %d, got %d",
			0,
			order.FilledQuantity(),
		)
	}
}

func TestOrderApplyInvalidFillQuantityFails(t *testing.T) {
	tests := []struct {
		name string
		fill Quantity
	}{
		{
			name: "zero fill",
			fill: Quantity(0),
		},
		{
			name: "negative fill",
			fill: Quantity(-1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := mustNewTestOrder(t)

			openedAt := order.CreatedAt().Add(time.Second)

			if err := order.Open(openedAt); err != nil {
				t.Fatalf("opening order: %v", err)
			}

			originalUpdatedAt := order.UpdatedAt()

			err := order.ApplyFill(
				tt.fill,
				openedAt.Add(time.Second),
			)

			if !errors.Is(err, ErrInvalidFillQuantity) {
				t.Fatalf(
					"expected error %v, got %v",
					ErrInvalidFillQuantity,
					err,
				)
			}

			if order.FilledQuantity() != 0 {
				t.Errorf(
					"expected filled quantity to remain %d, got %d",
					0,
					order.FilledQuantity(),
				)
			}

			if order.Status() != OrderStatusOpen {
				t.Errorf(
					"expected status to remain %q, got %q",
					OrderStatusOpen,
					order.Status(),
				)
			}

			if !order.UpdatedAt().Equal(originalUpdatedAt) {
				t.Errorf(
					"expected updated at to remain %v, got %v",
					originalUpdatedAt,
					order.UpdatedAt(),
				)
			}
		})
	}
}

func TestOrderApplyFillAfterCancellationFails(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	cancelledAt := openedAt.Add(time.Second)

	if err := order.Cancel(cancelledAt); err != nil {
		t.Fatalf("cancelling order: %v", err)
	}

	err := order.ApplyFill(
		Quantity(1_000_000),
		cancelledAt.Add(time.Second),
	)

	if !errors.Is(err, ErrInvalidOrderTransition) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrInvalidOrderTransition,
			err,
		)
	}

	if order.FilledQuantity() != 0 {
		t.Errorf(
			"expected filled quantity to remain %d, got %d",
			0,
			order.FilledQuantity(),
		)
	}

	if order.Status() != OrderStatusCancelled {
		t.Errorf(
			"expected status to remain %q, got %q",
			OrderStatusCancelled,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(cancelledAt) {
		t.Errorf(
			"expected updated at to remain %v, got %v",
			cancelledAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderApplyConsecutivePartialFills(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	firstFillAt := openedAt.Add(time.Second)

	if err := order.ApplyFill(
		Quantity(1_000_000),
		firstFillAt,
	); err != nil {
		t.Fatalf("applying first partial fill: %v", err)
	}

	secondFillAt := firstFillAt.Add(time.Second)

	if err := order.ApplyFill(
		Quantity(1_000_000),
		secondFillAt,
	); err != nil {
		t.Fatalf("applying second partial fill: %v", err)
	}

	expectedFilled := Quantity(2_000_000)

	if order.FilledQuantity() != expectedFilled {
		t.Errorf(
			"expected filled quantity %d, got %d",
			expectedFilled,
			order.FilledQuantity(),
		)
	}

	expectedRemaining := Quantity(3_000_000)

	if order.RemainingQuantity() != expectedRemaining {
		t.Errorf(
			"expected remaining quantity %d, got %d",
			expectedRemaining,
			order.RemainingQuantity(),
		)
	}

	if order.Status() != OrderStatusPartiallyFilled {
		t.Errorf(
			"expected status %q, got %q",
			OrderStatusPartiallyFilled,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(secondFillAt) {
		t.Errorf(
			"expected updated at %v, got %v",
			secondFillAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderApplyFillAfterFilledFails(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	filledAt := openedAt.Add(time.Second)

	if err := order.ApplyFill(order.Quantity(), filledAt); err != nil {
		t.Fatalf("filling order: %v", err)
	}

	originalUpdatedAt := order.UpdatedAt()

	err := order.ApplyFill(
		Quantity(1),
		filledAt.Add(time.Second),
	)

	if !errors.Is(err, ErrInvalidOrderTransition) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrInvalidOrderTransition,
			err,
		)
	}

	if order.Status() != OrderStatusFilled {
		t.Errorf(
			"expected status to remain %q, got %q",
			OrderStatusFilled,
			order.Status(),
		)
	}

	if order.FilledQuantity() != order.Quantity() {
		t.Errorf(
			"expected filled quantity %d, got %d",
			order.Quantity(),
			order.FilledQuantity(),
		)
	}

	if !order.UpdatedAt().Equal(originalUpdatedAt) {
		t.Errorf(
			"expected updated at to remain %v, got %v",
			originalUpdatedAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderApplyFillWithZeroTimeFails(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	originalUpdatedAt := order.UpdatedAt()

	err := order.ApplyFill(
		Quantity(1_000_000),
		time.Time{},
	)

	if !errors.Is(err, ErrOrderUpdateTimeRequired) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrOrderUpdateTimeRequired,
			err,
		)
	}

	if order.FilledQuantity() != 0 {
		t.Errorf(
			"expected filled quantity to remain %d, got %d",
			0,
			order.FilledQuantity(),
		)
	}

	if order.Status() != OrderStatusOpen {
		t.Errorf(
			"expected status to remain %q, got %q",
			OrderStatusOpen,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(originalUpdatedAt) {
		t.Errorf(
			"expected updated at to remain %v, got %v",
			originalUpdatedAt,
			order.UpdatedAt(),
		)
	}
}

func TestOrderConsecutivePartialFillWithOlderTimeFails(t *testing.T) {
	order := mustNewTestOrder(t)

	openedAt := order.CreatedAt().Add(time.Second)

	if err := order.Open(openedAt); err != nil {
		t.Fatalf("opening order: %v", err)
	}

	firstFillAt := openedAt.Add(time.Second)

	if err := order.ApplyFill(
		Quantity(1_000_000),
		firstFillAt,
	); err != nil {
		t.Fatalf("applying first fill: %v", err)
	}

	originalFilledQuantity := order.FilledQuantity()
	originalUpdatedAt := order.UpdatedAt()

	olderTime := firstFillAt.Add(-time.Second)

	err := order.ApplyFill(
		Quantity(1_000_000),
		olderTime,
	)

	if !errors.Is(err, ErrOrderUpdateTimeBeforeCurrent) {
		t.Fatalf(
			"expected error %v, got %v",
			ErrOrderUpdateTimeBeforeCurrent,
			err,
		)
	}

	if order.FilledQuantity() != originalFilledQuantity {
		t.Errorf(
			"expected filled quantity to remain %d, got %d",
			originalFilledQuantity,
			order.FilledQuantity(),
		)
	}

	if order.Status() != OrderStatusPartiallyFilled {
		t.Errorf(
			"expected status to remain %q, got %q",
			OrderStatusPartiallyFilled,
			order.Status(),
		)
	}

	if !order.UpdatedAt().Equal(originalUpdatedAt) {
		t.Errorf(
			"expected updated at to remain %v, got %v",
			originalUpdatedAt,
			order.UpdatedAt(),
		)
	}
}
