package orders

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrMatchingNilOrderBook = errors.New("order book cannot be nil")
	ErrMatchingTimeRequired = errors.New("matching time is required")
)

type MatchingEngine struct {
	nextTradeSequence uint64
}

func NewMatchingEngine() *MatchingEngine {
	return &MatchingEngine{}
}

func (e *MatchingEngine) Match(
	book *OrderBook,
	at time.Time,
) ([]*Trade, error) {
	if book == nil {
		return nil, ErrMatchingNilOrderBook
	}

	if at.IsZero() {
		return nil, ErrMatchingTimeRequired
	}

	at = at.UTC()

	trades := make([]*Trade, 0)

	for {
		bids := book.Bids()
		asks := book.Asks()

		if len(bids) == 0 || len(asks) == 0 {
			break
		}

		bestBid := bids[0]
		bestAsk := asks[0]

		if bestBid.Price() < bestAsk.Price() {
			break
		}

		quantity := bestBid.RemainingQuantity()

		if bestAsk.RemainingQuantity() < quantity {
			quantity = bestAsk.RemainingQuantity()
		}

		bidEntry := book.bids[bestBid.ID()]
		askEntry := book.asks[bestAsk.ID()]

		executionPrice := bestAsk.Price()

		if bidEntry.sequence < askEntry.sequence {
			executionPrice = bestBid.Price()
		}

		bidFillTime, err := bestBid.validateFill(quantity, at)
		if err != nil {
			return trades, fmt.Errorf(
				"validate bid %s: %w",
				bestBid.ID(),
				err,
			)
		}

		askFillTime, err := bestAsk.validateFill(quantity, at)
		if err != nil {
			return trades, fmt.Errorf(
				"validate ask %s: %w",
				bestAsk.ID(),
				err,
			)
		}

		tradeID := fmt.Sprintf(
			"trade-%06d",
			e.nextTradeSequence+1,
		)

		trade, err := NewTrade(
			tradeID,
			bestBid.Symbol(),
			bestBid.ID(),
			bestAsk.ID(),
			executionPrice,
			quantity,
			at,
		)

		if err != nil {
			return trades, fmt.Errorf(
				"create trade: %w",
				err,
			)
		}

		bestBid.applyValidatedFill(
			quantity,
			bidFillTime,
		)

		bestAsk.applyValidatedFill(
			quantity,
			askFillTime,
		)

		e.nextTradeSequence++

		trades = append(trades, trade)

		if bestBid.Status() == OrderStatusFilled {
			delete(book.bids, bestBid.ID())
		}

		if bestAsk.Status() == OrderStatusFilled {
			delete(book.asks, bestAsk.ID())
		}
	}

	return trades, nil
}
