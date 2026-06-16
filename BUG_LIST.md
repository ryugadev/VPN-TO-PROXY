# Bug List

## Fixed During Benchmark Hang Investigation

### BENCH-001: Benchmark simulator could wait indefinitely

- Severity: High
- Status: Fixed
- Area: `cmd/benchmark`
- Root cause: benchmark used HTTP requests and WebSocket dial/read/write calls without explicit deadlines. `simulateCommands` waited on `sync.WaitGroup` with no global timeout, so a stuck HTTP command request could block the whole run.
- Fix:
  - Added full benchmark timeout: 10 minutes.
  - Added agent registration timeout: 30 seconds.
  - Added WebSocket connect timeout: 15 seconds.
  - Added command dispatch/lifecycle timeout: 120 seconds.
  - Added command lifecycle polling and partial timeout reports.
  - Added progress logging every 10 seconds.
- Verification:
  - Benchmark now exits cleanly and generated `BENCHMARK_REPORT.md`.

### BENCH-002: Agent WebSocket benchmark hit default API rate limit

- Severity: High
- Status: Fixed
- Area: `internal/api/handler.go`
- Root cause: `/ws/agent` did not match the higher agent-control rate limit scope. During the 10 + 50 + 100 agent benchmark, the 121st WebSocket connection hit the default 120/minute limit, causing the 100-agent batch to connect only 60/100 agents.
- Fix:
  - Added `/ws/agent` to the `agent-control` rate limit scope with limit `2000/minute`.
- Verification:
  - Rerun connected 160/160 agents.
  - 500/500 commands completed.

## Open Issues

No critical benchmark-blocking bugs remain from this investigation.

## Latest Benchmark Summary

- Agents requested: 160
- Agents connected: 160
- Commands created: 500
- Commands completed: 500
- Failed operations: 0
- Timeout report: false
- SQLite busy/lock errors: 0
