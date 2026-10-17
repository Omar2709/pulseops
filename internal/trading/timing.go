package trading

import (
	"sync"
	"time"
)

// TimingOperation identifies the measured service action.
type TimingOperation string

const (
	TimingSubmit TimingOperation = "submit"
	TimingCancel TimingOperation = "cancel"
)

// TimingOutcome identifies the result of a measured operation.
type TimingOutcome string

const (
	TimingCommitted TimingOutcome = "committed"
	TimingReplayed  TimingOutcome = "replayed"
	TimingRejected  TimingOutcome = "rejected"
)

// OperationTiming captures the time spent waiting for service locks,
// persisting a state change and publishing in-memory state.
//
// Other work (validation, staging, matching, trade-ID assignment) is not
// included in these measurements. Durations are zero for phases not reached.
// Time within StoreApply includes the underlying database transaction.
type OperationTiming struct {
	Operation      TimingOperation
	Outcome        TimingOutcome
	SymbolLockWait time.Duration
	GlobalLockWait time.Duration
	StoreApply     time.Duration
	Publication    time.Duration
}

// TimingObserver receives measurements after all service locks are released.
// Implementations must be safe for concurrent Observe calls. Callbacks are
// synchronous and should be lightweight; slow callbacks add request latency.
type TimingObserver interface {
	Observe(OperationTiming)
}

// TimingObserverFunc adapts a function into a TimingObserver.
type TimingObserverFunc func(OperationTiming)

func (f TimingObserverFunc) Observe(timing OperationTiming) {
	f(timing)
}

func lockWithTiming(lock sync.Locker, enabled bool) time.Duration {
	if !enabled {
		lock.Lock()
		return 0
	}

	started := time.Now()
	lock.Lock()
	return time.Since(started)
}
