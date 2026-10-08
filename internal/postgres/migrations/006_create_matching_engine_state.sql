CREATE TABLE matching_engine_state (
	id SMALLINT PRIMARY KEY
		CHECK (id = 1),
	trade_sequence BIGINT NOT NULL
		CHECK (trade_sequence >= 0)
);

INSERT INTO matching_engine_state (
	id,
	trade_sequence
)
SELECT
	1,
	COALESCE(
		MAX(
			CASE
				WHEN id ~ '^trade-[0-9]+$'
				THEN substring(id FROM 7)::BIGINT
				ELSE 0
			END
		),
		0
	)
FROM trades;
