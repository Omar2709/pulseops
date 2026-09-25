package orders

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidOrderTransition       = errors.New("invalid order status transition")
	ErrOrderUpdateTimeRequired      = errors.New("order update time is required")
	ErrOrderUpdateTimeBeforeCurrent = errors.New("order update time cannot be before current update time")
	ErrInvalidFillQuantity          = errors.New("fill quantity must be greater than zero")
	ErrFillExceedsRemainingQuantity = errors.New("fill quantity exceeds remaining order quantity")
)

func (o *Order) Open(at time.Time) error {
	return o.transitionTo(OrderStatusOpen, at)
}

func (o *Order) Reject(at time.Time) error {
	return o.transitionTo(OrderStatusRejected, at)
}

func (o *Order) Cancel(at time.Time) error {
	return o.transitionTo(OrderStatusCancelled, at)
}

func (o *Order) transitionTo(next OrderStatus, at time.Time) error {
	if !o.status.CanTransitionTo(next) {
		return fmt.Errorf(
			"%w: %s -> %s",
			ErrInvalidOrderTransition,
			o.status,
			next,
		)
	}

	normalizedAt, err := o.validateUpdateTime(at)
	if err != nil {
		return err
	}

	o.status = next
	o.updatedAt = normalizedAt

	return nil
}

func (o *Order) ApplyFill(fill Quantity, at time.Time) error {
	normalizedAt, err := o.validateFill(fill, at)
	if err != nil {
		return err
	}

	o.applyValidatedFill(fill, normalizedAt)

	return nil
}

func (o *Order) validateFill(
	fill Quantity,
	at time.Time,
) (time.Time, error) {
	if fill <= 0 {
		return time.Time{}, ErrInvalidFillQuantity
	}

	if o.status != OrderStatusOpen &&
		o.status != OrderStatusPartiallyFilled {
		return time.Time{}, fmt.Errorf(
			"%w: cannot apply fill from %s",
			ErrInvalidOrderTransition,
			o.status,
		)
	}

	if fill > o.RemainingQuantity() {
		return time.Time{}, ErrFillExceedsRemainingQuantity
	}

	return o.validateUpdateTime(at)
}

func (o *Order) applyValidatedFill(
	fill Quantity,
	normalizedAt time.Time,
) {
	o.filledQuantity += fill

	if o.filledQuantity == o.quantity {
		o.status = OrderStatusFilled
	} else {
		o.status = OrderStatusPartiallyFilled
	}

	o.updatedAt = normalizedAt
}

func (o *Order) validateUpdateTime(at time.Time) (time.Time, error) {
	if at.IsZero() {
		return time.Time{}, ErrOrderUpdateTimeRequired
	}

	at = at.UTC()

	if at.Before(o.updatedAt) {
		return time.Time{}, ErrOrderUpdateTimeBeforeCurrent
	}

	return at, nil
}
