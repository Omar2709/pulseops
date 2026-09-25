# PulseOps

PulseOps is a trading and order-matching backend built with Go.

The project is being developed progressively to explore production-oriented backend engineering concepts including domain modeling, order matching, concurrency, transactional consistency, idempotency, PostgreSQL, Redis, observability, Docker, Kubernetes, and CI/CD.

The current implementation provides an in-memory, sequential matching engine. Controlled concurrent processing will be introduced in a later phase.

> PulseOps is an educational trading-system simulation. It is not intended for real-money trading.

## Current Status

PulseOps currently implements the core trading domain and an in-memory order-matching engine.

Implemented:

- BUY and SELL order sides
- Order lifecycle states
- Controlled order-state transitions
- Order cancellation and rejection
- Partial and complete order fills
- Multiple consecutive partial fills
- Remaining quantity calculation
- Fixed-point price representation
- Fixed-point quantity representation
- Encapsulated `Order` entity
- Order input validation and domain invariants
- UTC-normalized order timestamps
- `Trade` execution records
- In-memory `OrderBook`
- Separate bid and ask sides
- Order-book symbol isolation
- Price priority
- FIFO/time priority for orders at the same price
- Crossed-book detection
- Partial matching
- Matching against multiple counter-orders
- Resting-order execution price
- Coordinated order cancellation and order-book removal
- Validation of both counterparties before applying fills
- Unit tests using Go's standard testing package

The HTTP API, persistence layer, controlled concurrent processing, Redis integration, observability, and deployment infrastructure will be introduced progressively.

## Domain

### Order Side

Supported sides:

- `BUY`
- `SELL`

External string values are normalized before being converted into domain values.

### Order Status

Current order states:

- `PENDING`
- `OPEN`
- `PARTIALLY_FILLED`
- `FILLED`
- `CANCELLED`
- `REJECTED`

The domain controls which state transitions are valid.

Terminal states cannot transition back into active order states.

### Price

Prices use a fixed-point `int64` representation instead of floating-point values.

Current scale:

```text
1 monetary unit = 100 internal units
```

Example:

```text
60000.25
→ 6,000,025 internal units
```

This avoids binary floating-point arithmetic for monetary values.

### Quantity

Quantities also use a fixed-point `int64` representation.

Current scale:

```text
1 asset unit = 100,000,000 internal units
```

Example:

```text
0.05 BTC
→ 5,000,000 internal units
```

This avoids binary floating-point arithmetic for asset quantities.

### Order Lifecycle

Orders support controlled lifecycle operations:

```text
PENDING
├── OPEN
└── REJECTED

OPEN
├── PARTIALLY_FILLED
├── FILLED
└── CANCELLED

PARTIALLY_FILLED
├── FILLED
└── CANCELLED
```

A partially filled order can receive additional partial fills without changing its status:

```text
PARTIALLY_FILLED
      │
      │ additional partial fill
      ▼
PARTIALLY_FILLED
```

This is an update to the order's executed quantity, not a state transition.

Invalid transitions are rejected by the domain.

Examples include:

```text
PENDING → FILLED
OPEN → PENDING
PARTIALLY_FILLED → OPEN
FILLED → OPEN
FILLED → CANCELLED
CANCELLED → OPEN
REJECTED → OPEN
```

### Order Fills

An active order can receive partial or complete fills.

Example:

```text
Order quantity:
5,000,000

First fill:
2,000,000

Filled:
2,000,000

Remaining:
3,000,000

Status:
PARTIALLY_FILLED
```

A subsequent fill can complete the order:

```text
Previous filled quantity:
2,000,000

Second fill:
3,000,000

Total filled quantity:
5,000,000

Remaining:
0

Status:
FILLED
```

Fill quantities must be greater than zero and cannot exceed the order's remaining quantity.

Pending, cancelled, rejected, or fully filled orders cannot receive fills.

Validation is performed before mutating the order so failed fill operations leave the entity unchanged.

### Trade

A `Trade` represents a successful execution between a BUY order and a SELL order.

A trade contains:

```text
ID
Symbol
BuyOrderID
SellOrderID
Price
Quantity
ExecutedAt
```

Trade validation guarantees that:

- trade IDs are present
- symbols are present
- BUY and SELL order IDs are present
- BUY and SELL order IDs are different
- price is greater than zero
- quantity is greater than zero
- execution time is present

Execution timestamps are normalized to UTC.

### Timestamps

Order and trade timestamps are normalized internally to UTC.

Example:

```text
Input:
2026-09-25 10:30:00 UTC-5

        ↓

Stored:
2026-09-25 15:30:00 UTC
```

This preserves the same instant while keeping the internal representation consistent.

Failed lifecycle and fill operations do not modify `UpdatedAt`.

## Order Book

PulseOps includes an in-memory order book containing separate BUY and SELL sides.

```text
OrderBook
├── Bids
└── Asks
```

Only orders in the following states can enter the book:

```text
OPEN
PARTIALLY_FILLED
```

Orders with a different symbol from the order book are rejected.

Duplicate order IDs are also rejected.

### Price Priority

BUY orders prioritize the highest price:

```text
BUY 61,000
BUY 60,500
BUY 60,000
```

SELL orders prioritize the lowest price:

```text
SELL 60,000
SELL 60,500
SELL 61,000
```

### Time Priority

When multiple orders have the same price, the order added first receives priority.

Example:

```text
BUY 60,000 — first
BUY 60,000 — second
BUY 60,000 — third
```

Execution priority:

```text
first
second
third
```

This provides the initial price-time priority model used by the matching engine.

### Cancellation

Order cancellation is coordinated through the order book.

A successful cancellation:

```text
OPEN / PARTIALLY_FILLED
        ↓
     CANCELLED
        ↓
removed from OrderBook
```

This prevents cancelled orders from remaining as active liquidity inside the book.

## Matching Engine

PulseOps currently includes a sequential in-memory matching engine.

The engine repeatedly compares:

```text
best bid
vs
best ask
```

A trade can occur when:

```text
best bid >= best ask
```

If:

```text
best bid < best ask
```

the book is not crossed and no trade occurs.

### Execution Quantity

Trade quantity is the smaller remaining quantity between the two matching orders.

Example:

```text
BUY remaining:
5

SELL remaining:
2

Trade quantity:
2
```

The SELL order becomes `FILLED` while the BUY order remains `PARTIALLY_FILLED`.

### Multiple Counter-Orders

A larger order can match against multiple smaller orders.

Example:

```text
BUY 5 @ 61,000

SELL 2 @ 60,000
SELL 3 @ 60,500
```

Matching produces:

```text
Trade 1
quantity = 2
price = 60,000

Trade 2
quantity = 3
price = 60,500
```

The BUY order becomes fully filled after both executions.

### Execution Price

PulseOps currently uses the price of the resting order.

If the SELL order entered the book first:

```text
SELL 60,000
BUY 61,000
```

the execution price is:

```text
60,000
```

If the BUY order entered first:

```text
BUY 61,000
SELL 60,000
```

the execution price is:

```text
61,000
```

### Matching Safety

Before mutating either order, the engine validates that both counterparties can accept the fill.

Conceptually:

```text
validate BUY
    ↓
validate SELL
    ↓
create Trade
    ↓
apply BUY fill
    ↓
apply SELL fill
```

If one side is invalid, the engine returns an error without partially filling the valid counter-order.

This provides in-memory mutation safety for the current sequential implementation.

## Current Architecture

```text
pulseops/
├── cmd/
│   └── api/
│       └── main.go
│
├── internal/
│   └── orders/
│       ├── matching_engine.go
│       ├── matching_engine_test.go
│       ├── order.go
│       ├── order_test.go
│       ├── order_book.go
│       ├── order_book_test.go
│       ├── order_lifecycle.go
│       ├── order_lifecycle_test.go
│       ├── price.go
│       ├── price_test.go
│       ├── quantity.go
│       ├── quantity_test.go
│       ├── side.go
│       ├── side_test.go
│       ├── status.go
│       ├── status_test.go
│       ├── trade.go
│       └── trade_test.go
│
├── .gitattributes
├── .gitignore
├── go.mod
└── README.md
```

The `internal/orders` package currently contains the trading domain and the first in-memory matching implementation.

Responsibilities are separated as follows:

```text
order.go
→ Order entity and construction

order_lifecycle.go
→ order transitions, cancellation, and fills

trade.go
→ immutable trade execution records

order_book.go
→ in-memory bids, asks, ordering, and cancellation

matching_engine.go
→ matching crossed orders and producing trades
```

Tests are colocated with their corresponding domain components.

The architecture will continue to evolve as HTTP, persistence, concurrency, Redis, observability, and infrastructure are introduced.

## Current Limitations

The current matching engine intentionally prioritizes correctness and domain modeling over production-scale optimization.

Current limitations include:

- state is stored only in memory
- matching is sequential
- order-book data structures are not optimized for very large books
- trade IDs are generated using an in-memory sequence
- trade ID sequences are not durable across process restarts
- the order book is not currently safe for concurrent access from multiple goroutines
- no PostgreSQL persistence exists yet
- no Redis integration exists yet
- no external HTTP API exists yet

These limitations will be addressed progressively rather than adding infrastructure before the corresponding problem exists.

## Planned Evolution

PulseOps is planned to include:

- REST API using `net/http` and Chi
- BUY and SELL order endpoints
- Order lookup and cancellation endpoints
- Best bid / best ask market quotes
- PostgreSQL persistence
- Database migrations
- Transactional order and trade persistence
- Idempotency keys
- Redis
- Controlled matching-engine concurrency using goroutines and channels
- Race detector validation in CI
- Unit and integration tests
- Health and readiness endpoints
- Docker and Docker Compose
- Prometheus-compatible metrics
- Observability and tracing
- GitHub Actions
- Kubernetes manifests
- Architecture documentation

Technologies will be introduced only when the application reaches a problem that requires them.

## Requirements

- Go 1.27+

Check your installation:

```bash
go version
```

## Run

```bash
go run ./cmd/api
```

Current output:

```text
PulseOps trading service
```

The HTTP trading API has not been implemented yet.

## Development Validation

Format the code:

```bash
go fmt ./...
```

Run static analysis:

```bash
go vet ./...
```

Run all tests:

```bash
go test ./...
```

Run domain tests with coverage:

```bash
go test -cover ./internal/orders
```

Run verbose domain tests:

```bash
go test -v ./internal/orders
```

Check whitespace and formatting problems before committing:

```bash
git diff --check
```

The Go race detector will become part of the validation workflow once concurrent matching is introduced and CI runs in a compatible environment.

## Testing Philosophy

Tests focus primarily on domain behavior and invariants rather than implementation details.

Current tests cover behavior including:

- rejecting invalid order sides
- rejecting invalid prices
- rejecting invalid quantities
- protecting order-state transitions
- preventing terminal states from becoming active again
- preventing fills on inactive orders
- rejecting zero and negative fill quantities
- preventing fills that exceed remaining quantity
- supporting multiple consecutive partial fills
- calculating remaining quantity correctly
- normalizing symbols
- normalizing timestamps to UTC
- ensuring failed operations do not partially mutate orders
- validating `Trade` invariants
- accepting only active orders into the order book
- rejecting duplicate orders
- rejecting different symbols in the same order book
- removing and cancelling orders
- BUY price priority
- SELL price priority
- time priority at equal prices
- detecting uncrossed books
- complete fills
- partial fills
- matching one order against multiple counter-orders
- resting ASK execution price
- resting BID execution price
- matching-engine input validation
- protecting against partial mutation when a counter-order is invalid

Coverage is used as feedback rather than as the sole measure of test quality.

## Roadmap

The next major milestones are introducing the HTTP API and evolving the matching engine toward controlled concurrent processing.

Subsequent phases will introduce PostgreSQL persistence, transactional consistency, idempotency, Redis, observability, CI/CD, and deployment infrastructure.

## License

A license has not been selected yet.