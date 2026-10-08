package trading

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
)

type recordingStateStore struct {
	ctx    context.Context
	change StateChange
	err    error
}

func (s *recordingStateStore) Apply(
	ctx context.Context,
	change StateChange,
) error {
	s.ctx = ctx
	s.change = change

	return s.err
}

func TestSubmitOrderPersistenceFailureDoesNotPublishState(
	t *testing.T,
) {
	start := testTime()
	now := start

	service := newServiceWithStore(
		func() time.Time {
			return now
		},
		noopStateStore{},
	)

	_, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "sell-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting initial SELL: %v", err)
	}

	before, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting initial SELL: %v", err)
	}

	persistErr := errors.New("persistence unavailable")
	service.store = &recordingStateStore{err: persistErr}
	now = start.Add(time.Second)

	_, err = service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)

	if !errors.Is(err, persistErr) {
		t.Fatalf(
			"expected persistence error, got %v",
			err,
		)
	}

	after, err := service.GetOrder("sell-001")
	if err != nil {
		t.Fatalf("getting SELL after failure: %v", err)
	}

	if after != before {
		t.Error("existing order changed after persistence failure")
	}

	_, err = service.GetOrder("buy-001")
	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf(
			"failed incoming order was published: %v",
			err,
		)
	}

	book := service.books["BTCUSD"]
	if book == nil {
		t.Fatal("expected BTCUSD order book")
	}

	if got := len(book.Asks()); got != 1 {
		t.Errorf("expected 1 original ask, got %d", got)
	}

	if got := len(book.Bids()); got != 0 {
		t.Errorf("expected 0 bids after failure, got %d", got)
	}

	service.store = noopStateStore{}
	now = start.Add(2 * time.Second)

	result, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "buy-002",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting valid BUY after failure: %v", err)
	}

	if len(result.Trades) != 1 {
		t.Fatalf(
			"expected 1 trade, got %d",
			len(result.Trades),
		)
	}

	if got := result.Trades[0].ID(); got != "trade-000001" {
		t.Errorf(
			"expected trade-000001 after failed persistence, got %q",
			got,
		)
	}
}

type stateStoreContextKey string

func TestSubmitOrderPassesContextAndStateToStore(
	t *testing.T,
) {
	start := testTime()
	now := start

	service := newServiceWithStore(
		func() time.Time {
			return now
		},
		noopStateStore{},
	)

	_, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "sell-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideSell,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting initial SELL: %v", err)
	}

	recorder := &recordingStateStore{}
	service.store = recorder
	now = start.Add(time.Second)

	const key stateStoreContextKey = "request-id"

	ctx := context.WithValue(
		context.Background(),
		key,
		"request-123",
	)

	result, err := service.SubmitOrder(
		ctx,
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_100_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting matching BUY: %v", err)
	}

	if got := recorder.ctx.Value(key); got != "request-123" {
		t.Errorf("context value not propagated: %v", got)
	}

	if len(recorder.change.Orders) != 2 {
		t.Fatalf(
			"expected 2 persisted order snapshots, got %d",
			len(recorder.change.Orders),
		)
	}

	persisted := make(map[string]OrderSnapshot)

	for _, order := range recorder.change.Orders {
		persisted[order.ID] = order
	}

	for _, id := range []string{"sell-001", "buy-001"} {
		order, exists := persisted[id]
		if !exists {
			t.Errorf("missing persisted order %s", id)
			continue
		}

		if order.Status != orders.OrderStatusFilled {
			t.Errorf(
				"expected %s to be FILLED, got %s",
				id,
				order.Status,
			)
		}
	}

	if len(recorder.change.Trades) != 1 {
		t.Fatalf(
			"expected 1 persisted trade, got %d",
			len(recorder.change.Trades),
		)
	}

	if got := recorder.change.Trades[0].ID(); got != "trade-000001" {
		t.Errorf("unexpected persisted trade ID: %q", got)
	}

	if len(result.Trades) != 1 {
		t.Fatalf(
			"expected 1 returned trade, got %d",
			len(result.Trades),
		)
	}
}

func TestCancelOrderPersistenceFailureDoesNotPublishState(
	t *testing.T,
) {
	now := testTime()

	service := newServiceWithStore(
		func() time.Time {
			return now
		},
		noopStateStore{},
	)

	_, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting order: %v", err)
	}

	before, err := service.GetOrder("buy-001")
	if err != nil {
		t.Fatalf("getting order before cancellation: %v", err)
	}

	persistErr := errors.New("persistence unavailable")
	service.store = &recordingStateStore{err: persistErr}
	now = now.Add(time.Second)

	_, err = service.CancelOrder(
		context.Background(),
		"buy-001",
	)
	if !errors.Is(err, persistErr) {
		t.Fatalf(
			"expected persistence error, got %v",
			err,
		)
	}

	after, err := service.GetOrder("buy-001")
	if err != nil {
		t.Fatalf("getting order after failed cancellation: %v", err)
	}

	if after != before {
		t.Error("order changed after failed cancellation persistence")
	}

	book := service.books["BTCUSD"]
	if book == nil {
		t.Fatal("expected BTCUSD order book")
	}

	if got := len(book.Bids()); got != 1 {
		t.Errorf(
			"expected original bid to remain active, got %d",
			got,
		)
	}
}

func TestCancelOrderPassesContextAndStateToStore(
	t *testing.T,
) {
	now := testTime()

	service := newServiceWithStore(
		func() time.Time {
			return now
		},
		noopStateStore{},
	)

	_, err := service.SubmitOrder(
		context.Background(),
		SubmitOrderInput{
			ID:       "buy-001",
			Symbol:   "BTCUSD",
			Side:     orders.SideBuy,
			Price:    orders.Price(6_000_000),
			Quantity: orders.Quantity(5_000_000),
		},
	)
	if err != nil {
		t.Fatalf("submitting order: %v", err)
	}

	recorder := &recordingStateStore{}
	service.store = recorder
	now = now.Add(time.Second)

	const key stateStoreContextKey = "cancel-request-id"

	ctx := context.WithValue(
		context.Background(),
		key,
		"cancel-123",
	)

	cancelled, err := service.CancelOrder(
		ctx,
		"buy-001",
	)
	if err != nil {
		t.Fatalf("cancelling order: %v", err)
	}

	if got := recorder.ctx.Value(key); got != "cancel-123" {
		t.Errorf("context value not propagated: %v", got)
	}

	if len(recorder.change.Orders) != 1 {
		t.Fatalf(
			"expected 1 persisted order, got %d",
			len(recorder.change.Orders),
		)
	}

	persisted := recorder.change.Orders[0]

	if persisted.ID != "buy-001" ||
		persisted.Status != orders.OrderStatusCancelled {
		t.Errorf(
			"unexpected persisted cancellation: %+v",
			persisted,
		)
	}

	if len(recorder.change.Trades) != 0 {
		t.Errorf(
			"expected no cancellation trades, got %d",
			len(recorder.change.Trades),
		)
	}

	if cancelled != persisted {
		t.Error("returned cancellation differs from persisted snapshot")
	}
}
