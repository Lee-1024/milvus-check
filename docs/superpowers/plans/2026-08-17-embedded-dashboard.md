# Embedded Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a read-only, responsive Milvus monitoring dashboard to the existing `serve` process with no separate frontend build or deployment.

**Architecture:** A thread-safe dashboard store owns the latest successful report and refresh failure state. Embedded HTML/CSS/JavaScript assets render that snapshot through `/api/status`; the existing exporter collector and readiness endpoint consume the same store.

**Tech Stack:** Go `embed`, `net/http`, vanilla HTML/CSS/JavaScript, existing Milvus and Prometheus clients.

---

### Task 1: Shared Dashboard Snapshot

**Files:**
- Create: `internal/dashboard/store.go`
- Test: `internal/dashboard/store_test.go`

- [ ] Write tests proving a new store is not ready, successful updates return a defensive snapshot, and failures retain the last successful report while setting `up=false`.
- [ ] Run `go test ./internal/dashboard -run TestStore -v` and verify it fails because `NewStore` does not exist.
- [ ] Implement `Store`, `Snapshot`, `SetSuccess`, and `SetFailure` with `sync.RWMutex`.
- [ ] Run `go test ./internal/dashboard -run TestStore -v` and verify it passes.

Expected public shape:

```go
type Snapshot struct {
    Ready                  bool
    Up                     bool
    Report                 domain.CheckReport
    RefreshIntervalSeconds int64
    LastError              string
}

func NewStore(interval time.Duration) *Store
func (s *Store) SetSuccess(report domain.CheckReport)
func (s *Store) SetFailure(err error)
func (s *Store) Snapshot() Snapshot
```

### Task 2: Embedded HTTP Dashboard

**Files:**
- Create: `internal/dashboard/handler.go`
- Create: `internal/dashboard/assets/index.html`
- Create: `internal/dashboard/assets/app.css`
- Create: `internal/dashboard/assets/app.js`
- Test: `internal/dashboard/handler_test.go`

- [ ] Write handler tests for `/`, static assets, JSON content type, HTTP 503 before readiness, and HTTP 200 after a successful snapshot.
- [ ] Run `go test ./internal/dashboard -run TestHandler -v` and verify the missing handler causes failure.
- [ ] Implement embedded assets and a `Handler` that routes `/`, `/assets/app.css`, `/assets/app.js`, and `/api/status`.
- [ ] Build the dense responsive dashboard from the approved design using semantic HTML, CSS status tokens, DOM APIs, auto-refresh, manual refresh, stale/error states, and mobile record layout.
- [ ] Run `go test ./internal/dashboard -run TestHandler -v` and verify it passes.

The API response maps the domain report without exposing runtime configuration:

```go
type StatusResponse struct {
    Ready                  bool                      `json:"ready"`
    Up                     bool                      `json:"up"`
    Healthy                bool                      `json:"healthy"`
    CheckedAt              time.Time                 `json:"checked_at"`
    RefreshIntervalSeconds int64                     `json:"refresh_interval_seconds"`
    LastError              string                    `json:"last_error,omitempty"`
    Warnings               []string                  `json:"warnings,omitempty"`
    Collections            []domain.CollectionReport `json:"collections"`
}
```

### Task 3: Share Store With Exporter

**Files:**
- Modify: `internal/exporter/collector.go`
- Modify: `internal/exporter/collector_test.go`

- [ ] Update collector tests to construct a dashboard store, set success/failure snapshots, and assert `milvus_check_up` follows store state.
- [ ] Run `go test ./internal/exporter -v` and verify tests fail against the old constructor.
- [ ] Replace collector-owned report state with a `*dashboard.Store` dependency while preserving existing metric names and error counters.
- [ ] Run `go test ./internal/exporter -v` and verify it passes.

### Task 4: Wire Dashboard Into Serve

**Files:**
- Modify: `internal/cli/root.go`

- [ ] Create one dashboard store in `serve`, pass it to the exporter, and register dashboard routes on the existing mux.
- [ ] Replace the standalone readiness atomic with `store.Snapshot().Ready && store.Snapshot().Up`.
- [ ] Update the refresh loop to call `SetSuccess` and `SetFailure`.
- [ ] Run `go test ./...` and verify all packages pass.

### Task 5: Documentation And Verification

**Files:**
- Modify: `README.md`

- [ ] Document `/`, `/api/status`, automatic refresh, and the read-only scope.
- [ ] Run `gofmt -w cmd internal`.
- [ ] Run `go test ./...`, `go vet ./...`, `go build -o milvus-check.exe ./cmd/milvus-check`, and `git diff --check`.
- [ ] Start `serve` with the current configuration and verify the page at `http://127.0.0.1:2112/` in desktop, tablet, and mobile viewports.
- [ ] Confirm the table includes both `default/test001` and `test0000/embed_bge`, the page has no horizontal overflow, and `/api/status` contains no credentials.

## Self-Review

- Spec coverage: shared cache, API, embedded assets, summary metrics, collection details, error/stale states, responsive layout, security boundary, exporter integration, and browser verification are all represented.
- Placeholder scan: no deferred implementation placeholders are present.
- Type consistency: `dashboard.Store` is the single source used by handlers, readiness, refresh, and exporter collection.
