# Matching performance baseline

These benchmarks establish a repeatable baseline **before** changing the matching engine or OrderBook. They are not production capacity claims.

## What is measured

| Benchmark | Workload | Question |
| --- | --- | --- |
| `BenchmarkOrderBookTopOfBook` | Read the highest-priority bid from books of 100, 1,000, and 5,000 resting orders | How costly is sorting the book for every best-price lookup? |
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

## Optimization hypotheses

The current `Bids()` and `Asks()` APIs allocate and sort all active orders on every read; `MatchingEngine.Match` invokes both on every fill. That suggests a high cost for deep books and multi-fill orders. `OrderBook.Clone()` also scales with the number of resting orders. The benchmarks will help prioritize work; do not replace these structures without regression tests for price-time FIFO, partial fills, cancellation, rollback, global trade sequencing, and PostgreSQL recovery.
