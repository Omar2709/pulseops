# PulseOps

PulseOps is a concurrent trading and order-matching backend built with Go.

The project is being developed progressively to explore production-oriented backend engineering concepts including domain modeling, concurrency, transactional consistency, idempotency, PostgreSQL, Redis, observability, Docker, Kubernetes, and CI/CD.

> PulseOps is an educational trading-system simulation. It is not intended for real-money trading.

## Current Status

The project currently implements the foundations of the trading domain and core order lifecycle behavior.

Implemented:

* BUY and SELL order sides
* Order lifecycle states
* Valid order-state transitions
* Controlled order lifecycle transitions
* Order cancellation and rejection
* Partial and complete order fills
* Multiple consecutive partial fills
* Remaining quantity calculation
* Fixed-point price representation
* Fixed-point quantity representation
* Encapsulated `Order` entity
* Order input validation and domain invariants
* UTC-normalized order timestamps
* Unit tests using Go's standard testing package

The HTTP API, persistence layer, matching engine, Redis integration, and deployment infrastructure will be introduced progressively.

## Domain

### Order Side

Supported sides:

* `BUY`
* `SELL`

External string values are normalized before being converted into domain values.

### Order Status

Current order states:

* `PENDING`
* `OPEN`
* `PARTIALLY_FILLED`
* `FILLED`
* `CANCELLED`
* `REJECTED`

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

Orders currently support controlled lifecycle operations including:

```text
PENDING
  ├── OPEN
  └── REJECTED

OPEN
  ├── PARTIALLY_FILLED
  ├── FILLED
  └── CANCELLED

PARTIALLY_FILLED
  ├── PARTIALLY_FILLED
  ├── FILLED
  └── CANCELLED
```

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

An open order can receive partial or complete fills.

Example:

```text
Order quantity:
5,000,000

First fill:
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

Cancelled, rejected, pending, or fully filled orders cannot receive fills.

### Timestamps

Order timestamps are normalized internally to UTC.

For example:

```text
Input:
2026-09-16 12:00:00 UTC-5

NewOrder
   ↓

Stored:
2026-09-16 17:00:00 UTC
```

This preserves the same instant while keeping the internal representation consistent.

Failed lifecycle operations do not modify `UpdatedAt`.

## Current Architecture

```text
pulseops/
├── cmd/
│   └── api/
│       └── main.go
│
├── internal/
│   └── orders/
│       ├── order.go
│       ├── order_test.go
│       ├── price.go
│       ├── price_test.go
│       ├── quantity.go
│       ├── quantity_test.go
│       ├── side.go
│       ├── side_test.go
│       ├── status.go
│       └── status_test.go
│
├── .gitignore
├── go.mod
└── README.md
```

The architecture will evolve as infrastructure and additional domain capabilities are introduced.

## Planned Evolution

PulseOps is planned to include:

* REST API using `net/http` and Chi
* BUY and SELL order endpoints
* In-memory order book
* Bid and ask calculation
* Price-time priority matching engine
* Trade execution records
* Idempotency keys
* PostgreSQL persistence
* Redis
* Goroutines and channels for controlled concurrent processing
* Unit and integration tests
* Go race detector in CI
* Health and readiness endpoints
* Docker and Docker Compose
* Prometheus-compatible metrics and observability
* GitHub Actions
* Kubernetes manifests
* Architecture documentation

Technologies will be introduced only when the application reaches a problem that requires them.

## Requirements

* Go 1.27+

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

## Development Validation

Format the code:

```bash
go fmt ./...
```

Run static analysis:

```bash
go vet ./...
```

Run tests:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -cover ./internal/orders
```

Run verbose domain tests:

```bash
go test -v ./internal/orders
```

## Testing Philosophy

Tests focus primarily on domain behavior and invariants rather than implementation details.

Examples include:

* Rejecting invalid order sides
* Rejecting invalid prices
* Rejecting invalid quantities
* Protecting order state transitions
* Preventing terminal states from becoming active again
* Preventing fills on orders that cannot be executed
* Rejecting zero and negative fill quantities
* Preventing fills that exceed the remaining quantity
* Supporting multiple consecutive partial fills
* Calculating remaining quantity correctly
* Normalizing order symbols
* Normalizing timestamps to UTC
* Preventing invalid domain objects
* Ensuring failed operations do not mutate order state
* Ensuring failed operations do not modify timestamps

Coverage is used as feedback rather than as the sole measure of test quality.

## Roadmap

The next milestone is introducing trade execution records and an in-memory order book, followed by price-time priority matching.

Later phases will introduce the HTTP API, persistence, concurrency controls, Redis integration, observability, and deployment infrastructure.

## License

A license has not been selected yet.
