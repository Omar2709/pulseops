package trading

import (
	"context"

	"github.com/Omar2709/pulseops/internal/orders"
)

// StateChange contains the application state that must be persisted before
// the corresponding in-memory state is published.
type StateChange struct {
	Orders        []OrderSnapshot
	Trades        []*orders.Trade
	BookSequences map[string]uint64
	TradeSequence uint64

	// SkipTradeSequenceUpdate indicates that the service is committing a
	// change with no new trades and the durable global trade sequence
	// therefore does not need to be written. The service must ensure the
	// sequence was already persisted. Direct store callers default to false.
	SkipTradeSequenceUpdate bool
}

// RecoveredOrder contains one durable order and its active-book sequence.
type RecoveredOrder struct {
	Order        OrderSnapshot
	BookSequence *uint64
}

// RecoveryState contains the durable state required to rebuild the service.
type RecoveryState struct {
	Orders        []RecoveredOrder
	TradeSequence uint64
}

// StateStore persists one atomic application state change.
type StateStore interface {
	Apply(
		ctx context.Context,
		change StateChange,
	) error
}

// StateLoader loads durable state before the HTTP server begins serving.
type StateLoader interface {
	Load(
		ctx context.Context,
	) (RecoveryState, error)
}

// noopStateStore preserves the current in-memory-only runtime behavior.
type noopStateStore struct{}

func (noopStateStore) Apply(
	ctx context.Context,
	_ StateChange,
) error {
	return ctx.Err()
}
