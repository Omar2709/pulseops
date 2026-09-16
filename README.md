# PulseOps

PulseOps is a concurrent trading and order-matching backend built with Go.

The project is being developed progressively to explore production-oriented backend engineering concepts including domain modeling, concurrency, transactional consistency, idempotency, PostgreSQL, Redis, observability, Docker, Kubernetes, and CI/CD.

> PulseOps is an educational trading-system simulation. It is not intended for real-money trading.

## Current Status

The project currently implements the foundations of the trading domain.

Implemented:

* BUY and SELL order sides
* Order lifecycle states
* Valid order-state transitions
* Fixed-point price representation
* Fixed-point quantity representation
* Encapsulated Order entity
* Order input validation and domain invariants
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

This avoids using binary floating-point arithmetic for financial values.

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
* Order book and bid/ask calculation
* Price-time priority matching engine
* Partial and complete order fills
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

* rejecting invalid order sides
* rejecting invalid prices
* rejecting invalid order quantities
* protecting order state transitions
* normalizing order symbols
* preventing invalid domain objects

Coverage is used as feedback rather than as the sole measure of test quality.

## Roadmap

The next milestone is implementing order lifecycle behavior, including controlled state transitions, cancellation, partial fills, complete fills, and timestamp updates.

Later phases will introduce the in-memory order book and matching engine before adding HTTP and persistence.

## License

A license has not been selected yet.
