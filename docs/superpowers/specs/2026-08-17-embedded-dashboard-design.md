# Embedded Monitoring Dashboard Design

## Goal

Add a read-only monitoring dashboard to the existing `milvus-check serve` process. The dashboard shows the latest Milvus collection load checks and Prometheus-enriched metrics without a separate frontend project, runtime, or deployment.

## Selected Approach

Use embedded HTML, CSS, and vanilla JavaScript assets served by the Go HTTP server.

- Go embeds all page assets into the binary with `embed.FS`.
- `/` serves the dashboard shell.
- `/api/status` returns the latest cached `domain.CheckReport` as JSON.
- JavaScript loads `/api/status` on startup and refreshes it using `server.interval`.
- The existing `/metrics`, `/healthz`, and `/readyz` endpoints remain unchanged.
- The page has no write operations and cannot load, release, create, or modify collections.

This design avoids Node.js, a frontend build pipeline, CDN dependencies, and a separate service while still providing responsive updates and clear loading/error states.

## Data Flow

1. The existing refresh loop checks Milvus and queries Prometheus.
2. A thread-safe snapshot store records the latest report, refresh time, refresh interval, and last refresh error.
3. The Prometheus collector reads the same snapshot for exported metrics.
4. `/api/status` reads the snapshot and returns JSON without contacting Milvus synchronously.
5. The browser renders the snapshot and schedules the next refresh based on the interval returned by the API.

Serving cached data keeps the page responsive and prevents every browser refresh from creating a new Milvus and Prometheus workload.

## API Contract

`GET /api/status` returns HTTP 200 after at least one successful refresh:

```json
{
  "ready": true,
  "healthy": false,
  "checked_at": "2026-08-17T10:33:19+08:00",
  "refresh_interval_seconds": 30,
  "last_error": "",
  "collections": []
}
```

Before the first successful refresh it returns HTTP 503 with `ready: false`. After a later refresh failure it keeps the previous successful collection snapshot, reports `last_error`, and marks the service unavailable through `milvus_check_up` and `/readyz`.

The API never includes Milvus passwords, tokens, usernames, or Prometheus configuration.

## Page Structure

The first screen is the operational dashboard, not a landing page.

### Header

- Product name: `Milvus Check`
- Overall state: healthy, degraded, or unavailable
- Last successful check time
- Automatic refresh countdown
- Icon-only manual refresh button with an accessible label and tooltip

### Summary Band

Compact metrics show:

- database count
- collection count
- loaded collection count
- loading collection count
- not-loaded collection count
- error collection count
- search QPS
- query QPS
- failed requests per second

Prometheus values display `--` when the series is unavailable. Global QPS metrics are shown once and are not repeated for every collection.

### Collection Table

Each row shows:

- database
- collection
- load state text
- load progress bar and percentage
- entity count
- partition count
- index state
- Prometheus loaded entity count
- QueryNode segment count
- warning or error indicator

Rows are grouped and sorted by database and collection. Unloaded and failed collections appear visually prominent without relying on color alone.

### Empty And Error States

- Initial load: stable skeleton rows without layout shift.
- No collections: concise empty state.
- API failure: persistent error band while retaining the previous rendered snapshot.
- Stale data: visible stale indicator when the last successful check is older than twice the configured interval.

## Visual Direction

The page is a dense, quiet operations dashboard.

- Dark neutral background with white foreground text.
- Green for loaded, amber for loading, red for not loaded or failed, and gray for unavailable data.
- Status always includes text or an icon in addition to color.
- Compact typography suited to repeated scanning; no oversized hero text.
- Cards are used only for individual summary metrics, with a maximum 8px radius.
- No gradients, decorative blobs, nested cards, or marketing copy.
- Motion is limited to a subtle refresh rotation and short state transitions, disabled under `prefers-reduced-motion`.

## Responsive Behavior

- Desktop: summary metrics in a compact grid and a full-width table.
- Tablet: fewer summary columns and horizontally prioritized table fields.
- Mobile: collection rows become stacked record groups; secondary metric labels remain visible.
- Interactive targets are at least 44px and keyboard focus remains visible.

## File Boundaries

- `internal/dashboard/store.go`: thread-safe dashboard snapshot state.
- `internal/dashboard/handler.go`: `/`, embedded assets, and `/api/status` handlers.
- `internal/dashboard/assets/index.html`: semantic page structure.
- `internal/dashboard/assets/app.css`: responsive visual system.
- `internal/dashboard/assets/app.js`: fetch, rendering, countdown, and failure handling.
- `internal/dashboard/store_test.go`: snapshot and stale-state tests.
- `internal/dashboard/handler_test.go`: index and API response tests.
- `internal/cli/root.go`: wire the shared store into refresh, readiness, collector, and HTTP routes.
- `internal/exporter/collector.go`: consume shared snapshots instead of maintaining separate report state.

## Error Handling

- Dashboard failures do not stop `/metrics` or health endpoints.
- JSON encoding failures return HTTP 500 and are logged with request path and error.
- Refresh failures preserve the last successful snapshot and are visible in the API and page.
- Frontend rendering treats missing metric values as unavailable, never as zero.
- Browser code uses text nodes and DOM APIs for server-provided values to avoid HTML injection.

## Testing

- Store tests verify successful snapshots, failure preservation, readiness, and stale detection.
- Handler tests verify embedded index delivery, JSON content type, 503 before readiness, and absence of credentials.
- JavaScript is kept dependency-free and organized into small pure formatting/rendering helpers where practical.
- Go tests, `go vet`, build, and `git diff --check` must pass.
- Browser verification covers 1440x900, 768x1024, and 375x812 viewports, including loading, healthy, not-loaded, and API-error states.

## Scope Exclusions

- No authentication or authorization layer is added in this iteration.
- No collection load/release actions are exposed.
- No historical chart storage is added; Prometheus and Grafana remain responsible for time-series history.
- No frontend framework, package manager, or external font/icon CDN is introduced.
# Collection state filters

The `loading` and `not_load` summary cards act as toggle filters for the collection table. Each can be enabled independently; when both are enabled, the table shows collections matching either state. Clicking an enabled card removes that filter.

Filtering affects only the rendered collection rows and their pagination. Summary counts remain calculated from the complete Milvus snapshot. The active filter state survives background refreshes, and an empty filtered result shows a dedicated empty-state message.
