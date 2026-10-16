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
