package trading

import (
	"fmt"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

// NewRecoveredService rebuilds the in-memory registry, active books,
// and matching-engine sequence from durable state.
func NewRecoveredService(
	store StateStore,
	state RecoveryState,
) (*Service, error) {
	return newRecoveredService(
		func() time.Time {
			return time.Now().UTC()
		},
		store,
		state,
	)
}

func newRecoveredService(
	now func() time.Time,
	store StateStore,
	state RecoveryState,
) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("state store cannot be nil")
	}

	service := newServiceWithStore(now, store)
	service.engine = orders.NewMatchingEngineWithSequence(
		state.TradeSequence,
	)

	for _, recovered := range state.Orders {
		snapshot := recovered.Order

		if _, exists := service.orders[snapshot.ID]; exists {
			return nil, fmt.Errorf(
				"recover duplicate order %s",
				snapshot.ID,
			)
		}

		order, err := orders.RestoreOrder(
			snapshot.ID,
			snapshot.Symbol,
			snapshot.Side,
			snapshot.Price,
			snapshot.Quantity,
			snapshot.FilledQuantity,
			snapshot.Status,
			snapshot.CreatedAt,
			snapshot.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"recover order %s: %w",
				snapshot.ID,
				err,
			)
		}

		if snapshot.RemainingQuantity !=
			order.RemainingQuantity() {
			return nil, fmt.Errorf(
				"recover order %s: remaining quantity mismatch",
				snapshot.ID,
			)
		}

		active := order.Status() == orders.OrderStatusOpen ||
			order.Status() == orders.OrderStatusPartiallyFilled

		if active && recovered.BookSequence == nil {
			return nil, fmt.Errorf(
				"recover order %s: missing book sequence",
				snapshot.ID,
			)
		}

		if !active && recovered.BookSequence != nil {
			return nil, fmt.Errorf(
				"recover order %s: inactive order has book sequence",
				snapshot.ID,
			)
		}

		if active {
			book := service.books[order.Symbol()]
			if book == nil {
				book = orders.NewOrderBook()
				service.books[order.Symbol()] = book
			}

			if err := book.Restore(
				order,
				*recovered.BookSequence,
			); err != nil {
				return nil, fmt.Errorf(
					"recover order %s into book: %w",
					snapshot.ID,
					err,
				)
			}
		}

		service.orders[order.ID()] = order
	}

	return service, nil
}
