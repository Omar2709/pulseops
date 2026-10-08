package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Omar2709/pulseops/internal/orders"
	"github.com/Omar2709/pulseops/internal/trading"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPoolRequired = errors.New("PostgreSQL pool is required")

const upsertOrderSQL = `
INSERT INTO orders (
	id,
	symbol,
	side,
	price_units,
	quantity_units,
	filled_units,
	status,
	created_at,
	updated_at,
	book_sequence
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET
	filled_units = EXCLUDED.filled_units,
	status = EXCLUDED.status,
	updated_at = EXCLUDED.updated_at,
	book_sequence = EXCLUDED.book_sequence
WHERE orders.symbol = EXCLUDED.symbol
	AND orders.side = EXCLUDED.side
	AND orders.price_units = EXCLUDED.price_units
	AND orders.quantity_units = EXCLUDED.quantity_units
	AND orders.created_at = EXCLUDED.created_at
`

const insertTradeSQL = `
INSERT INTO trades (
	id,
	symbol,
	buy_order_id,
	sell_order_id,
	price_units,
	quantity_units,
	executed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`

type Store struct {
	pool *pgxpool.Pool
}

var _ trading.StateStore = (*Store)(nil)
var _ trading.StateLoader = (*Store)(nil)

func NewStore(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, ErrPoolRequired
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Apply(
	ctx context.Context,
	change trading.StateChange,
) error {
	if len(change.Orders) == 0 &&
		len(change.Trades) == 0 {
		return nil
	}

	if change.TradeSequence > math.MaxInt64 {
		return fmt.Errorf(
			"trade sequence exceeds PostgreSQL BIGINT",
		)
	}

	preparedSequences := make(
		map[string]*int64,
		len(change.Orders),
	)
	seenOrders := make(
		map[string]struct{},
		len(change.Orders),
	)

	for _, snapshot := range change.Orders {
		if _, exists := seenOrders[snapshot.ID]; exists {
			return fmt.Errorf(
				"duplicate order %s in state change",
				snapshot.ID,
			)
		}
		seenOrders[snapshot.ID] = struct{}{}

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
			return fmt.Errorf(
				"validate order %s: %w",
				snapshot.ID,
				err,
			)
		}

		if snapshot.RemainingQuantity !=
			order.RemainingQuantity() {
			return fmt.Errorf(
				"validate order %s: remaining quantity mismatch",
				snapshot.ID,
			)
		}

		sequence, hasSequence := change.BookSequences[snapshot.ID]

		active := snapshot.Status == orders.OrderStatusOpen ||
			snapshot.Status == orders.OrderStatusPartiallyFilled

		if active != hasSequence {
			return fmt.Errorf(
				"validate order %s: active state and book sequence disagree",
				snapshot.ID,
			)
		}

		if hasSequence {
			if sequence > math.MaxInt64 {
				return fmt.Errorf(
					"book sequence for order %s exceeds PostgreSQL BIGINT",
					snapshot.ID,
				)
			}

			value := int64(sequence)
			preparedSequences[snapshot.ID] = &value
		} else {
			preparedSequences[snapshot.ID] = nil
		}
	}

	for id := range change.BookSequences {
		if _, exists := seenOrders[id]; !exists {
			return fmt.Errorf(
				"book sequence references unchanged order %s",
				id,
			)
		}
	}

	err := pgx.BeginFunc(
		ctx,
		s.pool,
		func(tx pgx.Tx) error {
			for _, order := range change.Orders {
				tag, err := tx.Exec(
					ctx,
					upsertOrderSQL,
					order.ID,
					order.Symbol,
					string(order.Side),
					order.Price.Units(),
					order.Quantity.Units(),
					order.FilledQuantity.Units(),
					string(order.Status),
					postgresTime(order.CreatedAt),
					postgresTime(order.UpdatedAt),
					preparedSequences[order.ID],
				)
				if err != nil {
					return fmt.Errorf(
						"persist order %s: %w",
						order.ID,
						err,
					)
				}

				if tag.RowsAffected() != 1 {
					return fmt.Errorf(
						"persist order %s: immutable fields conflict",
						order.ID,
					)
				}
			}

			for _, trade := range change.Trades {
				tag, err := tx.Exec(
					ctx,
					insertTradeSQL,
					trade.ID(),
					trade.Symbol(),
					trade.BuyOrderID(),
					trade.SellOrderID(),
					trade.Price().Units(),
					trade.Quantity().Units(),
					postgresTime(trade.ExecutedAt()),
				)
				if err != nil {
					return fmt.Errorf(
						"persist trade %s: %w",
						trade.ID(),
						err,
					)
				}

				if tag.RowsAffected() != 1 {
					return fmt.Errorf(
						"persist trade %s: unexpected rows affected",
						trade.ID(),
					)
				}
			}

			tag, err := tx.Exec(
				ctx,
				`
					UPDATE matching_engine_state
					SET trade_sequence = $1
					WHERE id = 1
						AND trade_sequence <= $1
				`,
				int64(change.TradeSequence),
			)
			if err != nil {
				return fmt.Errorf(
					"persist matching engine sequence: %w",
					err,
				)
			}

			if tag.RowsAffected() != 1 {
				return fmt.Errorf(
					"persist matching engine sequence: sequence regression or missing state",
				)
			}

			return nil
		},
	)
	if err != nil {
		return fmt.Errorf(
			"apply PostgreSQL state change: %w",
			err,
		)
	}

	return nil
}

// Load reads durable orders, active-book sequences, and trade sequence.
func (s *Store) Load(
	ctx context.Context,
) (trading.RecoveryState, error) {
	rows, err := s.pool.Query(
		ctx,
		`
			SELECT
				id,
				symbol,
				side,
				price_units,
				quantity_units,
				filled_units,
				status,
				created_at,
				updated_at,
				book_sequence
			FROM orders
			ORDER BY id
		`,
	)
	if err != nil {
		return trading.RecoveryState{}, fmt.Errorf(
			"load orders: %w",
			err,
		)
	}
	defer rows.Close()

	recovered := make([]trading.RecoveredOrder, 0)

	for rows.Next() {
		var (
			id            string
			symbol        string
			sideText      string
			priceUnits    int64
			quantityUnits int64
			filledUnits   int64
			statusText    string
			createdAt     time.Time
			updatedAt     time.Time
			bookSequence  pgtype.Int8
		)

		if err := rows.Scan(
			&id,
			&symbol,
			&sideText,
			&priceUnits,
			&quantityUnits,
			&filledUnits,
			&statusText,
			&createdAt,
			&updatedAt,
			&bookSequence,
		); err != nil {
			return trading.RecoveryState{}, fmt.Errorf(
				"scan recovered order: %w",
				err,
			)
		}

		side, err := orders.ParseSide(sideText)
		if err != nil {
			return trading.RecoveryState{}, fmt.Errorf(
				"load order %s side: %w",
				id,
				err,
			)
		}

		price, err := orders.NewPrice(priceUnits)
		if err != nil {
			return trading.RecoveryState{}, fmt.Errorf(
				"load order %s price: %w",
				id,
				err,
			)
		}

		quantity, err := orders.NewQuantity(quantityUnits)
		if err != nil || quantity == 0 {
			return trading.RecoveryState{}, fmt.Errorf(
				"load order %s: invalid quantity",
				id,
			)
		}

		filled, err := orders.NewQuantity(filledUnits)
		if err != nil {
			return trading.RecoveryState{}, fmt.Errorf(
				"load order %s filled quantity: %w",
				id,
				err,
			)
		}

		status := orders.OrderStatus(statusText)

		order, err := orders.RestoreOrder(
			id,
			symbol,
			side,
			price,
			quantity,
			filled,
			status,
			createdAt,
			updatedAt,
		)
		if err != nil {
			return trading.RecoveryState{}, fmt.Errorf(
				"load order %s state: %w",
				id,
				err,
			)
		}

		var sequence *uint64
		if bookSequence.Valid {
			if bookSequence.Int64 < 0 {
				return trading.RecoveryState{}, fmt.Errorf(
					"load order %s: negative book sequence",
					id,
				)
			}

			value := uint64(bookSequence.Int64)
			sequence = &value
		}

		recovered = append(
			recovered,
			trading.RecoveredOrder{
				Order: trading.OrderSnapshot{
					ID:                order.ID(),
					Symbol:            order.Symbol(),
					Side:              order.Side(),
					Price:             order.Price(),
					Quantity:          order.Quantity(),
					FilledQuantity:    order.FilledQuantity(),
					RemainingQuantity: order.RemainingQuantity(),
					Status:            order.Status(),
					CreatedAt:         order.CreatedAt(),
					UpdatedAt:         order.UpdatedAt(),
				},
				BookSequence: sequence,
			},
		)
	}

	if err := rows.Err(); err != nil {
		return trading.RecoveryState{}, fmt.Errorf(
			"iterate recovered orders: %w",
			err,
		)
	}

	var tradeSequence int64

	if err := s.pool.QueryRow(
		ctx,
		`
			SELECT trade_sequence
			FROM matching_engine_state
			WHERE id = 1
		`,
	).Scan(&tradeSequence); err != nil {
		return trading.RecoveryState{}, fmt.Errorf(
			"load matching engine sequence: %w",
			err,
		)
	}

	if tradeSequence < 0 {
		return trading.RecoveryState{}, fmt.Errorf(
			"load matching engine sequence: negative value",
		)
	}

	return trading.RecoveryState{
		Orders:        recovered,
		TradeSequence: uint64(tradeSequence),
	}, nil
}

func postgresTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
