package trading

import (
	"context"

	"github.com/Omar2709/pulseops/internal/orders"
)

// StateChange contains the application state that must be persisted before
// the corresponding in-memory state is published.
type StateChange struct {
	Orders []OrderSnapshot
	Trades []*orders.Trade
}

// StateStore persists one atomic application state change.
type StateStore interface {
	Apply(
		ctx context.Context,
		change StateChange,
	) error
}

// noopStateStore preserves the current in-memory-only runtime behavior.
type noopStateStore struct{}

func (noopStateStore) Apply(
	context.Context,
	StateChange,
) error {
	return nil
}
