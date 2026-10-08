CREATE TABLE trades (
	id TEXT PRIMARY KEY,
	symbol TEXT NOT NULL
		CHECK (symbol <> '' AND symbol = UPPER(symbol)),
	buy_order_id TEXT NOT NULL
		REFERENCES orders(id) ON DELETE RESTRICT,
	sell_order_id TEXT NOT NULL
		REFERENCES orders(id) ON DELETE RESTRICT,
	price_units BIGINT NOT NULL
		CHECK (price_units > 0),
	quantity_units BIGINT NOT NULL
		CHECK (quantity_units > 0),
	executed_at TIMESTAMPTZ NOT NULL,
	CHECK (buy_order_id <> sell_order_id)
);
