CREATE TABLE orders (
	id TEXT PRIMARY KEY,
	symbol TEXT NOT NULL
		CHECK (symbol <> '' AND symbol = UPPER(symbol)),
	side TEXT NOT NULL
		CHECK (side IN ('BUY', 'SELL')),
	price_units BIGINT NOT NULL
		CHECK (price_units > 0),
	quantity_units BIGINT NOT NULL
		CHECK (quantity_units > 0),
	filled_units BIGINT NOT NULL
		CHECK (
			filled_units >= 0
			AND filled_units <= quantity_units
		),
	status TEXT NOT NULL
		CHECK (
			status IN (
				'PENDING',
				'OPEN',
				'PARTIALLY_FILLED',
				'FILLED',
				'CANCELLED',
				'REJECTED'
			)
		),
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
		CHECK (updated_at >= created_at)
);
