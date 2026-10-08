package orders

import (
	"errors"
	"fmt"
	"time"
)

var ErrOrderRestoreStateInvalid = errors.New("restored order state is invalid")

// RestoreOrder rebuilds an Order from validated durable state.
func RestoreOrder(
	id string,
	symbol string,
	side Side,
	price Price,
	quantity Quantity,
	filledQuantity Quantity,
	status OrderStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (*Order, error) {
	order, err := NewOrder(
		id,
		symbol,
		side,
		price,
		quantity,
		createdAt,
	)
	if err != nil {
		return nil, err
	}

	if updatedAt.IsZero() {
		return nil, ErrOrderUpdateTimeRequired
	}

	createdAt = createdAt.UTC()
	updatedAt = updatedAt.UTC()

	if updatedAt.Before(createdAt) {
		return nil, ErrOrderUpdateTimeBeforeCurrent
	}

	switch status {
	case OrderStatusPending:
		if filledQuantity != 0 ||
			!updatedAt.Equal(createdAt) {
			return nil, fmt.Errorf(
				"%w: invalid PENDING state",
				ErrOrderRestoreStateInvalid,
			)
		}

	case OrderStatusOpen:
		if filledQuantity != 0 {
			return nil, fmt.Errorf(
				"%w: OPEN order has fills",
				ErrOrderRestoreStateInvalid,
			)
		}

		if err := order.Open(updatedAt); err != nil {
			return nil, err
		}

	case OrderStatusPartiallyFilled,
		OrderStatusFilled:
		if err := order.Open(createdAt); err != nil {
			return nil, err
		}

		if err := order.ApplyFill(
			filledQuantity,
			updatedAt,
		); err != nil {
			return nil, err
		}

	case OrderStatusCancelled:
		if err := order.Open(createdAt); err != nil {
			return nil, err
		}

		if filledQuantity > 0 {
			if err := order.ApplyFill(
				filledQuantity,
				updatedAt,
			); err != nil {
				return nil, err
			}
		}

		if err := order.Cancel(updatedAt); err != nil {
			return nil, err
		}

	case OrderStatusRejected:
		if filledQuantity != 0 {
			return nil, fmt.Errorf(
				"%w: REJECTED order has fills",
				ErrOrderRestoreStateInvalid,
			)
		}

		if err := order.Reject(updatedAt); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf(
			"%w: unsupported status %q",
			ErrOrderRestoreStateInvalid,
			status,
		)
	}

	if order.Status() != status ||
		order.FilledQuantity() != filledQuantity ||
		!order.UpdatedAt().Equal(updatedAt) {
		return nil, fmt.Errorf(
			"%w: state does not match lifecycle invariants",
			ErrOrderRestoreStateInvalid,
		)
	}

	return order, nil
}
