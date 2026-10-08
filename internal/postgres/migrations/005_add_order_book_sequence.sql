ALTER TABLE orders
ADD COLUMN book_sequence BIGINT;

DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM orders
		WHERE status IN ('OPEN', 'PARTIALLY_FILLED')
	) THEN
		RAISE EXCEPTION
			'cannot enable exact order-book recovery while legacy active orders exist without persisted sequence';
	END IF;
END
$$;

ALTER TABLE orders
ADD CONSTRAINT orders_book_sequence_nonnegative
CHECK (
	book_sequence IS NULL
	OR book_sequence >= 0
);

ALTER TABLE orders
ADD CONSTRAINT orders_book_sequence_status_check
CHECK (
	(
		status IN ('OPEN', 'PARTIALLY_FILLED')
		AND book_sequence IS NOT NULL
	)
	OR
	(
		status NOT IN ('OPEN', 'PARTIALLY_FILLED')
		AND book_sequence IS NULL
	)
);

CREATE UNIQUE INDEX orders_symbol_book_sequence_idx
ON orders (symbol, book_sequence)
WHERE book_sequence IS NOT NULL;
