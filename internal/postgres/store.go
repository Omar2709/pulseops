package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Omar2709/pulseops/internal/trading"
	"github.com/jackc/pgx/v5"
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
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
	filled_units = EXCLUDED.filled_units,
	status = EXCLUDED.status,
	updated_at = EXCLUDED.updated_at
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

func postgresTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
