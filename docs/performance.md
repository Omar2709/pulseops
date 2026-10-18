# Matching performance baseline

These benchmarks provide a repeatable performance baseline for the matching engine and OrderBook, including the first best-price optimization. They are not production capacity claims.

## What is measured

| Benchmark | Workload | Question |
| --- | --- | --- |
| `BenchmarkOrderBookTopOfBook` | Read the highest-priority bid from books of 100, 1,000, and 5,000 resting orders through `Bids()` | What is the cost of a fully sorted snapshot? |
| `BenchmarkOrderBookBestBid` / `BenchmarkOrderBookBestAsk` | Scan 100, 1,000, and 5,000 orders for the best entry | How much does allocation-free price selection cost? |
| `BenchmarkOrderBookClone` | Clone books of 100, 1,000, and 5,000 orders | What memory/time overhead does transactional staging add? |
| `BenchmarkMatchingEngineCrossingBook` | Match one BUY across 100, 500, and 1,000 SELL orders | How does execution cost scale with number of fills? |
| `BenchmarkSubmitOrderBookDepth` | Submit a non-crossing BUY to books of depth 0, 100, and 1,000 | How expensive is the application-service path as depth grows? |
| `BenchmarkSubmitMatchingParallel` | Concurrent SELL/BUY pairs over 1 versus 8 symbols | Where does global commit serialization limit throughput? |

Book creation and fixture cloning are excluded from the measured interval where explicitly noted. Parallel submissions use the **in-memory** store; PostgreSQL network/transaction throughput is **not** measured here. The matching benchmark measures in-memory matching only.

## Run

Requires the Go version declared in `go.mod`. Run on the same machine, with similar CPU load, to compare results across commits.

```bash
go test -run '^$' -bench . -benchmem -count 3 -benchtime=200ms ./internal/orders ./internal/trading
```

To compare concurrency under controlled scheduling:

```bash
GOMAXPROCS=4 go test -run '^$' -bench '^BenchmarkSubmitMatchingParallel$' -benchmem -count 3 ./internal/trading
```

Run a focused smoke test (also in CI) to confirm that representative benchmarks execute:

```bash
go test -run '^$' -bench '^BenchmarkOrderBookTopOfBook$/^depth_100$' -benchtime=1x ./internal/orders
go test -run '^$' -bench '^BenchmarkSubmitOrderBookDepth$/^depth_0$' -benchtime=1x ./internal/trading
```

## Profiling

```bash
go test -run '^$' -bench '^BenchmarkMatchingEngineCrossingBook$/^trades_500$' -benchtime=3s -cpuprofile=cpu.out -memprofile=mem.out ./internal/orders
go tool pprof -top cpu.out
go tool pprof -alloc_space mem.out
```

The profile files remain local; do not commit them. To avoid comparing random noise, collect at least three samples and compare `ns/op`, `B/op`, and `allocs/op` with the same benchmark, Go version, OS, and `GOMAXPROCS`.

## Measured best-price optimization (October 2026)

The benchmark compared two commits on GitHub-hosted Linux runners with the same `BenchmarkMatchingEngineCrossingBook` fixture and command:

```bash
go test -run '^$' -bench '^BenchmarkMatchingEngineCrossingBook$' -benchtime=100ms -count=3 -benchmem ./internal/orders
```

- **Before:** commit `041ac76`, [CI run 38853450655](https://github.com/Omar2709/pulseops/actions/runs/38853450655), implementation sorting both sides for every fill.
- **After:** commit `53e9c00`, [CI run 38853663145](https://github.com/Omar2709/pulseops/actions/runs/38853663145), implementation scanning both sides for the best price while preserving sequence tie-breaking.

Mean of three samples (lower is better):

| Trades in one crossing order | Before | After | Speedup | Allocated per operation, before → after |
| ---: | ---: | ---: | ---: | ---: |
| 100 | 0.439 ms | 0.119 ms | 3.7× | ~154 KiB → ~15 KiB |
| 500 | 13.347 ms | 1.901 ms | 7.0× | ~3.22 MiB → ~76 KiB |
| 1,000 | 59.719 ms | 7.278 ms | 8.2× | ~12.61 MiB → ~170 KiB |

CI runners are not performance-isolated, so treat these as directional comparisons, not guarantees or formal statistical significance. The entire benchmark runs **in memory**, excludes fixture cloning from timing, and does not measure HTTP or PostgreSQL throughput.

## Non-crossing order staging optimization (October 2026)

`SubmitOrder` now distinguishes an incoming order that could cross resting liquidity from one that must remain in the book. Crossing submissions still deep-clone the working book and entities to support rollback. **Non-crossing** submissions clone only the map structure, retain existing unchanged order pointers, skip matching, and persist only the new order and its original FIFO sequence. Post-commit publication remains atomic. Cancellations continue using deep clones.

Benchmark command (in-memory `StateStore`, three repetitions):

```bash
go test -run '^$' -bench '^BenchmarkSubmitOrderBookDepth$/^depth_(100|1000)$' -benchtime=150ms -count=3 -benchmem ./internal/trading
```

- Baseline commit `e01eaeb`, [CI run 38854235873](https://github.com/Omar2709/pulseops/actions/runs/38854235873).
- Optimized commit `e6f8b41`, [CI run 38854461423](https://github.com/Omar2709/pulseops/actions/runs/38854461423).

Mean of three samples:

| Resting depth | Before | After | Speedup | Allocated before → after | Allocations before → after |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 15.69 μs/op | 9.13 μs/op | 1.7× | 42.3 KiB → 5.7 KiB | 121 → 15 |
| 1,000 | 179.27 μs/op | 58.36 μs/op | 3.1× | 472 KiB → 81 KiB | 1,027 → 17 |

These are directional microbenchmarks from separate GitHub Actions CI executions, not an isolated statistical comparison. Book fixture creation is excluded from measured time, while the submission's map copying and in-memory persistence are included. **This does not measure PostgreSQL TPS**, even though the new path reduces the number of rows upserted for non-crossing submissions.

## Next optimization hypotheses

`MatchingEngine.Match` scans each side for the best available entry without sorting or allocating a full ordered view. The public `Bids()` and `Asks()` APIs still sort when callers request a complete snapshot. Matching retains **O(n) best-price selection per fill**, so an indexed price-level structure could help deep books.

Non-crossing submissions still copy the active book maps, an **O(n)** step. Crossing submissions and cancellations still deep-clone all active orders. Further optimizations require careful validation of price-time FIFO, partial fills, cancellation, rollback, global trade sequencing and PostgreSQL recovery.

## PostgreSQL transaction baseline

The PostgreSQL-specific benchmark exercises real transactions and commits, not an in-memory store.
It uses TEST_DATABASE_URL, runs migrations, resets the tables and sequence before each sub-benchmark,
then inserts unique non-crossing orders through Store.Apply.

Run on an isolated disposable PostgreSQL database:

```bash
TEST_DATABASE_URL='postgres://...' go test -run '^$' -bench '^BenchmarkPostgresApplyResting$' -benchtime=150ms -count=3 -benchmem ./internal/postgres
```

Two scenarios are measured: serial (one goroutine) and parallel (testing.B.RunParallel,
multiple simultaneous writers sharing a pgx pool). This isolates PostgreSQL transaction
and global sequence-row contention, without measuring HTTP or matching.

**Destructive warning:** this benchmark truncates orders and trades, then resets the global
trade sequence. Never point TEST_DATABASE_URL to a database containing valuable data.

CI collects three samples per scenario. Differences on GitHub-hosted runners are indicative;
do not interpret them as production capacity or statistically controlled benchmarks.

## Reducing PostgreSQL global sequence-row contention (October 2026)

The trading service now marks non-crossing submissions and cancellations as changes that **do not advance the confirmed trade sequence**.
The PostgreSQL store skips the `matching_engine_state` UPDATE for those operations, so they can commit without acquiring that shared row lock.
Matched orders continue to update the global trade sequence in the same transaction as orders and trades. Direct callers of `Store.Apply`
retain the old behavior unless they explicitly opt in to `SkipTradeSequenceUpdate`. Opting in while inserting trades is rejected.

The benchmark exercises actual PostgreSQL writes with one writer and multiple parallel writers (`testing.B.RunParallel`).
Each reported value is the mean of three GitHub Actions samples, using a disposable PostgreSQL database and identical fixture.

| Workload | Before | After | Speedup | Approximate operations/s before → after |
| --- | ---: | ---: | ---: | ---: |
| Serial writer | 477.6 μs/op | 430.3 μs/op | 1.11× | 2,094 → 2,324 |
| Concurrent writers | 256.3 μs/op | 176.4 μs/op | 1.45× | 3,901 → 5,670 |

- Baseline: commit `3132c1a`, [CI run 38855287429](https://github.com/Omar2709/pulseops/actions/runs/38855287429).
- Optimized: commit `c2f343d`, [CI run 38855619401](https://github.com/Omar2709/pulseops/actions/runs/38855619401).

**Interpretation:** Concurrent `ns/op` reflects aggregate throughput per operation, not per-request latency.
Because GitHub-hosted runners are shared, these figures are directional, not controlled production benchmarks.
The benchmark directly calls `Store.Apply`, so it isolates the database layer; the trading service still serializes final commits
under its global mutex, and its real end-to-end TPS may be lower. PostgreSQL has not been load-tested over a remote network.

An integration test holds a PostgreSQL row lock on the trade sequence while submitting and cancelling resting orders.
It asserts that those operations complete before the lock is released, then verifies the next matched trade correctly persists sequence 1.

## End-to-end trading service with PostgreSQL

Use `BenchmarkTradingServicePostgresMatch` to measure the trading application
service plus actual PostgreSQL transactions and commits, rather than just
`Store.Apply`. Each benchmark operation submits a resting SELL followed by a
crossing BUY and commits **one trade**. It includes in-process per-symbol
staging, global commit locking, real PostgreSQL writes, and sequence updates.
HTTP transport, networked clients, and remote-database latency are excluded.

Three cases distinguish serial submissions from parallel submissions that
target one symbol or eight independent symbols. The `parallel_*` `ns/op`
values represent amortized throughput per completed SELL/BUY pair,
not the individual latency of each request. Final trade count and global
sequence are checked after the measurement.

```bash
TEST_DATABASE_URL='postgres://...' go test -run '^$' -bench '^BenchmarkTradingServicePostgresMatch$' -benchtime=150ms -count=3 -benchmem ./internal/postgres
```

**Destructive:** the shared benchmark fixture truncates `orders` and `trades`
and resets the global trade sequence for every scenario and sample. Only
run on a disposable PostgreSQL database. The benchmark skips when
`TEST_DATABASE_URL` is unset.

GitHub Actions collects three samples. Report performance measurements with
their workflow run and Go/host environment. Use the existing in-memory and
direct PostgreSQL benchmarks to distinguish matching, persistence, and
global commit contention; avoid changing the serialization model solely
based on an unisolated shared-runner benchmark.

### First end-to-end PostgreSQL results (October 2026)

Data from commit `614fad7` in [GitHub Actions run 39079884435](https://github.com/Omar2709/pulseops/actions/runs/39079884435).
Each measured operation is a pair of submissions (one resting SELL, one crossing BUY) that commits exactly one trade via `trading.Service` and `Store.Apply`.
The measurements include symbol gate contention, book staging, global commit mutex, and two PostgreSQL transactions.

| Scenario | Mean of 3 samples | Approx. completed pairs/s |
| --- | ---: | ---: |
| Serial | 2.130 ms/pair | 469 |
| Parallel, 1 symbol | 2.227 ms/pair | 449 |
| Parallel, 8 symbols | 2.251 ms/pair | 444 |

**Finding:** More independent symbols did not increase aggregate confirmed-trade throughput in this sample.
The service still holds a global write lock while assigning trade IDs, persisting the state change, and publishing in-memory state.
This appears to dominate over independent per-symbol staging for the tested short-lived matched pairs.
It is **not proof that this is the only bottleneck**; variations between benchmark subtests on shared CI workers are not statistically controlled.

**Next engineering decision:** Before changing global locking, design a commit coordinator that preserves globally unique monotonic trade IDs, persistence-before-publication, rollback, and order-ID uniqueness across symbols.
Instrument separately (a) per-symbol lock wait, (b) global commit lock wait, (c) PostgreSQL Apply time, and (d) publication time.
Only then consider allowing nonmatching commits on separate symbols to overlap, and retain global serialization for trade sequence allocation.
Any concurrency change requires stress and race tests, deterministic recovery tests, and benchmarks measuring durable trade correctness rather than throughput alone.

## Optional service phase instrumentation

Use `trading.NewServiceWithStoreAndTimingObserver(store, observer)` to
measure the critical stages of each order submission and cancellation.
The `TimingObserver` is opt-in; the existing constructors remain unchanged.

- `SymbolLockWait`: time spent waiting to acquire the symbol mutex.
- `GlobalLockWait`: time waiting for the global commit mutex.
- `StoreApply`: elapsed time executing the StateStore's Apply call, including
  SQL statements and transaction commit in PostgreSQL mode.
- `Publication`: time publishing committed state to the order registry,
  affected book, and global trade sequence in memory.

Observers run synchronously **after all service locks are released**. They
must be safe for concurrent calls and should aggregate rather than retain
unbounded per-request events. Any blocking observer adds response latency
but does not hold service locks. The outcome is `committed`, `replayed`,
or `rejected`. Operations returning before acquiring the symbol lock,
such as cancelling a nonexistent order, are not observed.

The PostgreSQL matched-pair benchmark uses an observer and emits
`symbol_wait_ns/submit`, `global_wait_ns/submit`,
`store_apply_ns/submit`, and `publication_ns/submit`.
These values are **averages per SubmitOrder call**; the benchmark's
`ns/op` measures one SELL+BUY pair. They do not sum to total time because
validation, matching, state staging, trade allocation and observer overhead
are outside the four phases. The benchmark measures a local PostgreSQL
service inside GitHub Actions; it does not estimate production network latency.

```bash
TEST_DATABASE_URL='postgres://...' go test -run '^$' -bench '^BenchmarkTradingServicePostgresMatch$' -benchtime=150ms -count=3 -benchmem ./internal/postgres
```

As with the other database benchmarks, the fixture truncates test tables
and resets the trade counter. Run it only against a disposable database.

### Phase results with actual PostgreSQL (October 17, 2026)

Instrumented [CI run 39080729177](https://github.com/Omar2709/pulseops/actions/runs/39080729177),
commit `9e1afb9`. Means of three samples in Go benchmark, **per SubmitOrder**:

| Phase | Serial | 8 concurrent symbols |
| --- | ---: | ---: |
| Symbol gate wait | 0.081 μs | 0.148 μs |
| Global commit lock wait | 0.096 μs | 2,527 μs |
| Store.Apply duration | 1,109 μs | 1,265 μs |
| State publication | 0.898 μs | 0.940 μs |

These are overlapping observations from concurrent goroutines, so the phase
averages do **not** add to `ns/op` in parallel benchmarks. The one-symbol
parallel case has a benchmark-level pair mutex, which intentionally prevents
interleaving and makes it less useful for measuring commit-lock contention.
The eight-symbol case exhibits a dramatic increase in wait time on the
global mutex compared with the serial case, while the symbol wait stays tiny.
The PostgreSQL `Apply` phase is by far the dominant time **inside the
globally serialized section** for these short-lived matched pairs.

**Next step:** design a safe commit coordinator or separate durable write
sequencing. Do not simply unlock around Apply: this could break the global
trade-ID sequence, order-ID uniqueness across symbols and the invariant
that memory publishes only successfully committed durable state.

## Parallel durable commits for independent non-crossing orders (October 2026)

After the phase instrumentation showed high global mutex wait under eight concurrent symbols,
the service now removes the global lock from `StateStore.Apply` **only for a non-crossing
order**. Each affected symbol still holds its own mutex throughout staging and commit.
Global order-ID reservations (`pendingOrderIDs`, under `s.mu`) prevent duplicate IDs
on different symbols while persistence is in progress. Reservations are cleared after
success or error. The commit is published under `s.mu` after persistence succeeds.

Matched-trade submissions and cancellations retain their existing globally serialized
commit paths; a delayed resting-order publication never replaces an engine sequence
advanced by another symbol. This is **single-process** coordination and does not
enable horizontally scaled independent matching processes.

### PostgreSQL benchmark comparison

The same `BenchmarkTradingServicePostgresResting` was run against the previous
globally serialized implementation and the optimized implementation, using
three samples per scenario in disposable PostgreSQL services on GitHub Actions.
Each `ns/op` is the amortized time for one durable, non-crossing submission.

| Scenario | Before, median of 3 | After, median of 3 | Ratio |
| --- | ---: | ---: | ---: |
| Serial | 776.6 μs/op | 386.7 μs/op | ~2.0× |
| Parallel, 1 symbol | 750.7 μs/op | 396.1 μs/op | ~1.9× |
| Parallel, 8 symbols | 771.5 μs/op | 142.9 μs/op | ~5.4× |

- Previous code + benchmark: commit `736f0c2`, [CI run 39164823832](https://github.com/Omar2709/pulseops/actions/runs/39164823832).
- New parallel resting writes: commit `2889681`, [CI run 39164709166](https://github.com/Omar2709/pulseops/actions/runs/39164709166).

In the eight-symbol sample the global-mutex wait reported by the timing observer
fell from a median ~1.53 ms/submit to ~0.45 μs/submit. One baseline sample
experienced a large slowdown (~2.99 ms/op), so **medians are reported** rather
than an outlier-skewed average. CI runners are shared, and even the serial
comparison varies considerably; treat these results as directional, not as
statistically controlled speedups or production throughput commitments.

Book depth grows with the number of submissions, and tests exclude fixture
setup from the measured interval. HTTP traffic and remote PostgreSQL latency
are not measured. The benchmark validates committed order count and ensures
the trade sequence remains unchanged; separate integration tests exercise
concurrent trade commits and recovery across delayed resting publications.

### Correctness boundaries

- `pendingOrderIDs` rejects a conflicting same-ID submission on a different
  symbol while the original durable write remains pending.
- Replays on the **same** symbol still wait for its symbol mutex and return
  the committed current snapshot after successful publication.
- Persistence failures release ID reservations and do not publish the order.
- A running non-crossing commit can overlap a trade commit for a different
  symbol; the trade sequence remains durable and monotonic.
- Cancellation and matching retain their global commit lock; this change
  does not optimize their serial persistence path.
