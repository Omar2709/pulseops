# PulseOps

PulseOps is a trading and order-matching backend built with Go.

The project is being developed progressively to explore production-oriented backend engineering concepts including domain modeling, order matching, concurrency, transactional consistency, idempotency, PostgreSQL, Redis, observability, Docker, Kubernetes, and CI/CD.

The current implementation provides an in-memory trading application service, a sequential matching engine, and an HTTP API for submitting, retrieving, and cancelling limit orders. Order submission and cancellation provide persistence-before-publication semantics through a pluggable StateStore. When DATABASE_URL is configured, PostgreSQL stores order snapshots and trades transactionally and rebuilds the order registry, exact active-book FIFO sequences, and matching-engine trade sequence during startup; without it, the service keeps the existing in-memory-only behavior. Submission, matching, and cancellation are serialized using an exclusive lock, while order lookup uses a read lock; controlled concurrent matching will be introduced in a later phase.

> PulseOps is an educational trading-system simulation. It is not intended for real-money trading.

## Current Status

PulseOps currently implements the core trading domain, an in-memory order-matching engine, HTTP order submission with in-memory atomicity for matching errors, order lookup by ID, and HTTP order cancellation.

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
- HTTP server using Go's `net/http`
- Standard-library `ServeMux` routing
- `GET /healthz` liveness endpoint
- JSON response helpers
- Standardized JSON error responses
- HTTP handler tests using `net/http/httptest`
- In-memory trading application service
- Order submission and duplicate ID detection
- Order snapshots to prevent external entity mutation
- Order lookup by ID
- Current order-state retrieval after matching
- Detached order snapshots that protect internal entity state
- Injectable application clock
- In-memory order registry
- Independent order books per symbol
- Reader-writer synchronization using `sync.RWMutex`
- In-memory atomic order submission using independent working copies
- `OrderBook.Clone()` preserving active order state and original FIFO insertion sequences
- `MatchingEngine.Clone()` preserving the confirmed trade-ID sequence
- Deferred publication of updated orders, the order book, and the matching engine after successful matching
- Failed matching leaves the order registry, active book, and confirmed trade-ID sequence unchanged
- Pluggable `StateStore` persistence boundary for order submission and cancellation
- Request context propagation from HTTP operations into the persistence boundary
- Persistence failures leave in-memory orders, books, and trade-ID sequence unpublished
- PostgreSQL-backed StateStore using pgx v5
- Transactional order upserts and trade inserts
- Embedded, tracked PostgreSQL schema migrations
- Optional PostgreSQL runtime configuration through DATABASE_URL
- PostgreSQL integration tests in CI
- Exact PostgreSQL startup recovery of the order registry
- Persisted FIFO book sequences for active orders
- Durable matching-engine trade sequence recovery
- Recovery validation through domain lifecycle invariants
- Fixed-point decimal parsing without floating-point arithmetic
- `POST /v1/orders` endpoint
- `GET /v1/orders/{id}` endpoint
- `DELETE /v1/orders/{id}` cancellation endpoint
- Application-service cancellation of open and partially filled orders
- Exclusive synchronization of cancellation with matching
- Retention of cancelled orders and their executed quantities in the order registry
- HTTP request validation and body size limits
- HTTP integration tests for order submission and matching
- HTTP integration tests for order lookup
- HTTP integration tests for cancellation, partial fills, error responses, and method restrictions
- Regression tests for failed matching before and after an initial trade
- Regression tests for trade-ID continuity after a failed submission
- Tests for order-book clone isolation and preservation of cross-side insertion sequences
- GitHub Actions CI for formatting, static analysis, uncached tests, and Linux race detection

Persistence, controlled concurrent matching, Redis integration, observability, and deployment infrastructure will be introduced progressively.

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

### Transactional Cloning

`OrderBook.Clone()` creates independent copies of the active orders, preserving their insertion sequence numbers and the book's `nextSequence`. It also returns a map of all cloned active orders. This map retains references to working orders that become fully filled and leave the working book during matching, allowing their updated states to be committed to the service registry.

The clone currently relies on `Order` containing value fields. If mutable maps, slices, or pointers are added to the entity, the cloning strategy must be reviewed to maintain isolation.

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

This validates both sides before applying each individual trade. `MatchingEngine.Match()` itself updates the book and orders in place and does not independently roll back an entire batch after a later failure. For application-level order submission, `trading.Service` performs matching on cloned working state and publishes the result only if the complete matching operation succeeds.

`MatchingEngine.Clone()` also copies the current trade-ID sequence. Trade IDs generated in an unsuccessful working match are discarded without advancing the confirmed engine.

## Current Architecture

```text
pulseops/
├── .github/
│   └── workflows/
│       └── ci.yml
│
├── cmd/
│   └── api/
│       └── main.go
│
├── internal/
│   ├── httpapi/
│   │   ├── decimal.go
│   │   ├── decimal_test.go
│   │   ├── health.go
│   │   ├── health_test.go
│   │   ├── orders.go
│   │   ├── orders_delete.go
│   │   ├── orders_delete_test.go
│   │   ├── orders_get.go
│   │   ├── orders_get_test.go
│   │   ├── orders_test.go
│   │   ├── response.go
│   │   ├── response_test.go
│   │   ├── router.go
│   │   └── router_test.go
│   │
│   ├── postgres/
│   │   ├── migrate.go
│   │   ├── pool.go
│   │   ├── store.go
│   │   ├── store_integration_test.go
│   │   └── migrations/
│   │       ├── 001_create_orders.sql
│   │       ├── 002_create_trades.sql
│   │       ├── 003_create_trades_buy_index.sql
│   │       ├── 004_create_trades_sell_index.sql
│   │       ├── 005_add_order_book_sequence.sql
│   │       └── 006_create_matching_engine_state.sql
│   │
│   ├── orders/
│   │   ├── matching_engine.go
│   │   ├── matching_engine_test.go
│   │   ├── order.go
│   │   ├── order_restore.go
│   │   ├── order_restore_test.go
│   │   ├── order_test.go
│   │   ├── order_book.go
│   │   ├── order_book_clone_test.go
│   │   ├── order_book_recovery_test.go
│   │   ├── order_book_test.go
│   │   ├── order_lifecycle.go
│   │   ├── order_lifecycle_test.go
│   │   ├── price.go
│   │   ├── price_test.go
│   │   ├── quantity.go
│   │   ├── quantity_test.go
│   │   ├── side.go
│   │   ├── side_test.go
│   │   ├── status.go
│   │   ├── status_test.go
│   │   ├── trade.go
│   │   └── trade_test.go
│   │
│   └── trading/
│       ├── recovery.go
│       ├── recovery_test.go
│       ├── service.go
│       ├── service_test.go
│       ├── store.go
│       └── store_test.go
│
├── .gitattributes
├── .gitignore
├── LICENSE
├── go.mod
├── go.sum
└── README.md
```

The `internal/httpapi` package contains HTTP routing, decimal parsing, handlers, and response helpers. The `internal/orders` package contains the trading domain, the in-memory matching engine, and independent cloning support for both the active order book and the engine. The `internal/trading` package coordinates atomic in-memory order submission, matching, synchronized order lookup, and cancellation through an in-memory application service.

Responsibilities are separated as follows:

```text
github/workflows/ci.yml
→ Linux CI for formatting, go vet, uncached tests, and the Go race detector
httpapi/router.go
→ standard-library HTTP routing with ServeMux
httpapi/health.go
→ GET /healthz liveness handler
httpapi/orders.go
→ POST /v1/orders validation, submission, and shared order response mapping for POST, GET, and DELETE
httpapi/orders_delete.go
→ DELETE /v1/orders/{id}, cancellation and HTTP error mapping
httpapi/orders_get.go
→ GET /v1/orders/{id}, lookup and HTTP response mapping
httpapi/decimal.go
→ strict fixed-point decimal parsing without float64
httpapi/response.go
→ JSON response and standardized error helpers
trading/service.go
→ order registry, independent symbol books, transactional working copies for submission, persistence-before-publication coordination, synchronized order lookup and cancellation, and snapshots
trading/store.go
→ persistence port for atomic application state changes without coupling the service to PostgreSQL
postgres/pool.go
→ pgxpool creation, DATABASE_URL parsing, connection timeout, and startup connectivity validation
postgres/migrate.go
→ embedded tracked migrations executed transactionally under a PostgreSQL advisory lock
postgres/store.go
→ PostgreSQL StateStore and StateLoader with transactional writes plus startup recovery of orders, FIFO sequences, and trade sequence
order.go
→ Order entity and construction
order_lifecycle.go
→ order transitions, cancellation, and fills
trade.go
→ immutable trade execution records
order_book.go
→ in-memory bids, asks, ordering, cancellation, and Clone() with preserved FIFO and insertion sequences
order_book_clone_test.go
→ cloned-book isolation, insertion sequence retention, and cross-side priority tests
matching_engine.go
→ matching crossed orders, producing trades, and Clone() with the current trade-ID sequence
```

Tests are colocated with their corresponding HTTP, application-service, and domain components.

For order submission, `trading.Service` locks the shared state, creates a working book with copies of its active orders, and clones the matching engine. It stages the incoming order and runs matching only against these working objects. A matching error discards all working state; success commits updated order references (including fully filled orders removed from the working book), the incoming order, the resulting book, and the advanced engine before releasing the exclusive lock. The book and registry therefore reference the same committed `Order` instances for active orders.

Before publishing a successful submission or cancellation in memory, `trading.Service` sends the resulting order snapshots and trades to its `StateStore`. A store error aborts publication, preserving the previously committed in-memory book, registry, and trade-ID sequence. HTTP request contexts are propagated into this boundary so PostgreSQL work observes cancellation and deadlines.

When `DATABASE_URL` is configured, the PostgreSQL StateStore wraps every StateChange in one database transaction: all order snapshots are inserted or updated first, followed by trade inserts. Any failure rolls back the entire state change. Order IDs cannot silently overwrite different immutable order data. Migrations are embedded in the binary, tracked in `schema_migrations`, and run transactionally at startup under an advisory lock.

The in-memory registry and order books remain the runtime source used by GET and matching, but PostgreSQL mode now rebuilds them before the HTTP server starts. Active orders recover their persisted book sequence rather than deriving priority from timestamps, and the matching engine resumes from its durable trade sequence. A committed database change therefore survives a single-process crash and is reflected after restart. Safe multi-process matching is still future work because each process owns an independent in-memory book.

The architecture will continue to evolve with persistence, controlled concurrent matching, Redis, observability, and infrastructure.

## HTTP API

### Submit Order

`POST /v1/orders`

The endpoint accepts limit orders using decimal strings for price and quantity. Binary floating-point arithmetic is not used.

Example request:

```json
{
  "id": "buy-001",
  "symbol": "BTCUSD",
  "side": "BUY",
  "price": "60000.25",
  "quantity": "0.05"
}
```

Successful requests return `201 Created`. If no compatible counter-order is available, the new order remains `OPEN`; otherwise, the matching engine may partially or completely fill it.

Example response when no compatible counter-order is available (timestamps vary):

```json
{
  "id": "buy-001",
  "symbol": "BTCUSD",
  "side": "BUY",
  "price_units": "6000025",
  "quantity_units": "5000000",
  "filled_units": "0",
  "remaining_units": "5000000",
  "status": "OPEN",
  "created_at": "2026-10-02T14:00:28Z",
  "updated_at": "2026-10-02T14:00:28Z"
}
```

Prices and quantities are returned as fixed-point internal units. These values are encoded as JSON strings to preserve integer precision across different HTTP clients. Responses are snapshots reflecting the state at the time of submission; they do not update when later orders are matched.

The endpoint supports automatic matching against compatible resting orders within the same symbol.

Validation includes malformed JSON, unknown fields, invalid domain values, duplicate order IDs, unsupported media types, and oversized request bodies. The request body limit is 16 KiB. Common responses include:

- `201 Created`: accepted order, including its state after the current matching attempt
- `400 Bad Request`: malformed JSON or invalid request/domain values
- `405 Method Not Allowed`: unsupported HTTP method for this route
- `409 Conflict`: duplicate order ID
- `413 Request Entity Too Large`: request body exceeds the limit
- `415 Unsupported Media Type`: request is not sent as `application/json`

### Get Order

`GET /v1/orders/{id}`

Retrieves the current state of an order using its unique identifier.

Example request:

```http
GET /v1/orders/buy-001
```

Successful requests return `200 OK` and use the same JSON response format as the order submission endpoint.

Unlike the response returned when an order is submitted, this endpoint provides the order's current state, including any fills caused by subsequent matching operations.

Orders are retrieved from the application service's order registry. Fully executed orders remain available even after being removed from the active order book.

The application service uses `sync.RWMutex` to coordinate concurrent reads with order submissions and matching.

Responses:

- `200 OK`: the order exists.
- `404 Not Found`: the order does not exist.
- `405 Method Not Allowed`: the HTTP method is not supported.

Go's standard `ServeMux` also accepts `HEAD` requests for routes registered with `GET`.

Order lookup returns snapshots instead of exposing mutable domain entities. In-memory-only mode loses state on restart; PostgreSQL mode restores persisted orders into the runtime registry before serving requests.

Authentication and authorization are not yet implemented. This API must not be exposed publicly in its current form.

### Cancel Order

`DELETE /v1/orders/{id}`

Cancels an existing order by its unique identifier. Only orders in `OPEN` or `PARTIALLY_FILLED` state can be cancelled. Cancellation removes the order from the active order book but retains its latest state in the application registry, where it remains available through `GET /v1/orders/{id}`.

Example request:

```http
DELETE /v1/orders/buy-001
```

A successful request returns `200 OK` and the same JSON representation used by order submission and lookup, with `status` set to `CANCELLED`. For a partially filled order, `filled_units` preserves the quantity already executed and `remaining_units` records the unfilled quantity at cancellation. The remaining quantity is historical information and is no longer available for matching.

Example response for a partially filled order (timestamps and values are illustrative):

```json
{
  "id": "buy-001",
  "symbol": "BTCUSD",
  "side": "BUY",
  "price_units": "6000000",
  "quantity_units": "5000000",
  "filled_units": "2000000",
  "remaining_units": "3000000",
  "status": "CANCELLED",
  "created_at": "2026-10-03T14:00:00Z",
  "updated_at": "2026-10-03T14:00:02Z"
}
```

Responses:

- `200 OK`: the order was cancelled; returns the updated order snapshot.
- `404 Not Found`: no order exists with the supplied ID.
- `409 Conflict`: the order is already cancelled, fully filled, or otherwise not cancellable.
- `405 Method Not Allowed`: the HTTP method is not supported. The `Allow` header includes `GET`, `HEAD`, and `DELETE` for this order-resource route.
- `500 Internal Server Error`: an unexpected internal failure occurred.

The application service holds an exclusive `sync.RWMutex` lock during cancellation, preventing concurrent order submission or matching through that service. Cancellation is staged on a working book and persisted before publication. In PostgreSQL mode, a committed cancellation is recovered after a single-process restart.

Runtime state is held in memory; PostgreSQL mode restores its durable state after restart, while in-memory-only mode does not. Authentication and authorization are not implemented, so this API must not be exposed publicly in its current form.

## Current Limitations

The current matching engine intentionally prioritizes correctness and domain modeling over production-scale optimization.

Current limitations include:

- Runtime matching state is still held in memory, but PostgreSQL mode rebuilds it at startup from durable orders, active-book sequences, and the matching-engine sequence.
- Matching is sequential.
- Order submission, matching, and cancellation currently use an exclusive lock; order lookup uses a shared read lock.
- The order book is not safe for unsynchronized concurrent access outside the application service.
- Order submission provides in-memory atomicity for errors returned during matching by using independent working copies of the affected order book, its active orders, and the matching engine.
- The default runtime remains in-memory when DATABASE_URL is unset; only PostgreSQL mode provides durable restart recovery.
- Each submission clones the active orders of its symbol, which introduces additional memory and processing costs for large order books.
- Order-book data structures are not optimized for very large books.
- Trade IDs use an in-memory sequence during execution; PostgreSQL mode persists and restores that sequence, while in-memory-only mode resets it on restart.
- PostgreSQL recovery is single-process only; cross-process matching coordination is not implemented. Redis integration does not exist yet.
- Authentication and authorization have not yet been implemented.
- The HTTP API is intended for educational simulation, not production trading.
- Linux CI is configured to run the Go race detector with CGO enabled. Local Windows race-detector execution still requires CGO and a compatible C compiler.

- Migration 005 intentionally refuses to infer FIFO priority for legacy active orders that were persisted before book sequences existed; those active rows must be drained or cleared before that migration can be applied.

These limitations will be addressed progressively rather than adding infrastructure before the corresponding problem exists.

## Planned Evolution

PulseOps is planned to include:

- Additional REST endpoints using `net/http` (with optional routing libraries if needed)
- Best bid / best ask market quotes
- Multi-process matching coordination
- Evaluation of incremental or otherwise more efficient staging for large order books
- Idempotency keys
- Redis
- Controlled matching-engine concurrency using goroutines and channels
- Unit and integration tests
- Health and readiness endpoints
- Docker and Docker Compose
- Prometheus-compatible metrics
- Observability and tracing
- Kubernetes manifests
- Architecture documentation

Technologies will be introduced only when the application reaches a problem that requires them.

## Requirements

- Go 1.27+
- PostgreSQL 18.x when persistent mode is enabled

Check your installation:

```bash
go version
```

## Run

Without `DATABASE_URL`, PulseOps keeps its in-memory-only development mode:

```bash
go run ./cmd/api
```

To enable PostgreSQL persistence, set the connection string through the environment instead of embedding credentials in code:

```powershell
$env:DATABASE_URL = "postgres://USER:PASSWORD@localhost:5432/pulseops?sslmode=disable"
go run ./cmd/api
```

When PostgreSQL mode is enabled, startup validates the connection, applies pending embedded migrations, loads durable orders and sequencing state, and rebuilds the in-memory service before the HTTP server starts. Do not use `sslmode=disable` for remote or production database connections.

This starts the HTTP server. The `GET /healthz` liveness endpoint, `POST /v1/orders` submission endpoint, `GET /v1/orders/{id}` lookup endpoint, and `DELETE /v1/orders/{id}` cancellation endpoint are available on port `8080` by default.

To submit an example order from PowerShell:

```powershell
$body = @{
    id       = "buy-001"
    symbol   = "BTCUSD"
    side     = "BUY"
    price    = "60000.25"
    quantity = "0.05"
} | ConvertTo-Json

Invoke-RestMethod `
    -Uri "http://localhost:8080/v1/orders" `
    -Method POST `
    -ContentType "application/json" `
    -Body $body
```

To retrieve the order's latest state:

```powershell
Invoke-RestMethod `
    -Uri "http://localhost:8080/v1/orders/buy-001" `
    -Method GET
```

To cancel an open or partially filled order:

```powershell
Invoke-RestMethod `
    -Uri "http://localhost:8080/v1/orders/buy-001" `
    -Method DELETE
```

The in-memory-only registry resets when the process restarts. PostgreSQL mode restores the registry, active order books with exact FIFO priority, and the confirmed trade sequence before serving requests. Reusing an order ID during one process lifetime returns `409 Conflict`.

## Development Validation

Format the code:

```bash
go fmt ./...
```

Run static analysis:

```bash
go vet ./...
```

Run all tests without cached results:

```bash
go test -count=1 ./...
```

Run the in-memory atomicity regression and clone-isolation tests:

```bash
go test -run "TestSubmitOrder(MatchingFailureDoesNotChangeState|FailureAfterFirstTradeDoesNotChangeState|FailureDoesNotConsumeTradeSequence)$" -count=1 -v ./internal/trading
go test -run "^TestOrderBookClone" -count=1 -v ./internal/orders
```

Run domain tests with coverage:

```bash
go test -cover ./internal/orders
```

Run verbose domain tests:

```bash
go test -v ./internal/orders
```

Run HTTP integration tests and measure HTTP package coverage:

```bash
go test -v ./internal/httpapi
go test -cover ./internal/httpapi
```

Run application-service tests and measure service coverage:

```bash
go test -v ./internal/trading
go test -cover ./internal/trading
```

When CGO is enabled and a compatible C compiler is available, run the Go race detector:

```bash
go test -race ./...
```

Passing concurrent unit tests does not establish that the program is race-free. The Linux CI workflow runs the race detector independently from the normal test job.

### Continuous Integration

GitHub Actions validates pushes to `main`, pull requests, and manual workflow runs.

The CI pipeline verifies:

- module metadata with `go mod tidy`
- Go formatting without silently committing formatter changes
- `go vet ./...`
- uncached unit and integration tests with `go test -count=1 ./...`
- PostgreSQL integration tests against PostgreSQL 18.6
- `go test -race -count=1 ./...` on Linux with CGO enabled

The workflow uses read-only repository permissions and pins third-party action references to immutable commit SHAs.

Check whitespace and formatting problems before committing:

```bash
git diff --check
git diff --cached --check
```

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
- `TestSubmitOrderMatchingFailureDoesNotChangeState`: failed incoming orders leave no registry or book changes
- `TestSubmitOrderFailureAfterFirstTradeDoesNotChangeState`: a later matching failure discards earlier provisional fills
- `TestSubmitOrderFailureDoesNotConsumeTradeSequence`: failed matching does not consume confirmed trade IDs
- `TestSubmitOrderPersistenceFailureDoesNotPublishState`: persistence errors do not publish provisional state or consume trade IDs
- `TestSubmitOrderPassesContextAndStateToStore`: request context, updated order snapshots, and trades reach the persistence boundary
- cancellation persistence failures do not publish cancelled state
- cancellation request context and cancelled snapshots reach the persistence boundary
- PostgreSQL migrations are repeatable and tracked
- PostgreSQL StateStore persists orders and trades atomically
- PostgreSQL StateStore rolls back the entire StateChange when a trade insert fails
- PostgreSQL StateStore rejects immutable order-ID conflicts
- order lifecycle restoration rejects impossible persisted states
- order-book restoration preserves persisted FIFO sequence exactly
- PostgreSQL restart recovery preserves active-order priority and the next trade ID
- `TestOrderBookClonePreservesPriorityAndIsolation`: cloned books preserve FIFO and do not share mutable orders
- `TestOrderBookClonePreservesCrossSideSequence`: clone preserves insertion priority across BUY and SELL sides
- `GET /healthz` handler behavior
- JSON response helpers
- standardized JSON error responses
- HTTP routing using `net/http/httptest`
- fixed-point decimal parsing and overflow boundaries
- HTTP order submission and fixed-point JSON serialization
- HTTP input validation and expected status codes
- duplicate order rejection through HTTP
- matching across successive HTTP requests
- order lookup returning the current state after matching
- order lookup returning 404 for unknown identifiers
- order lookup rejecting unsupported HTTP methods with 405
- HTTP cancellation of open orders, followed by lookup of the cancelled state
- HTTP rejection of nonexistent and fully filled order cancellations
- HTTP cancellation of partially filled orders without undoing prior fills
- prevention of further matching after HTTP cancellation
- HTTP cancellation route rejecting unsupported methods with 405
- snapshot isolation across repeated order lookups
- concurrent application-service order submission

Coverage is used as feedback rather than as the sole measure of test quality.

## Roadmap

In-memory atomicity for matching errors is implemented using independent working copies and deferred publication. PostgreSQL mode adds transactional durable writes and startup recovery for a single service process.

The PostgreSQL-backed StateStore, schema migrations, transactional order/trade writes, cancellation persistence, exact active-book FIFO recovery, and durable trade-sequence recovery are implemented. The next milestone is controlled concurrent matching and evaluation of more efficient staging for large books. Subsequent phases will introduce idempotency, Redis, observability, deployment automation, and Kubernetes infrastructure.

## License

This project is licensed under the MIT License. See the `LICENSE` file for details.
