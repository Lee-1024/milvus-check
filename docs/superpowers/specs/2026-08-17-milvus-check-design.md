# Milvus Collection Check Design

## Goal

Build a Go command line and exporter tool for Milvus 2.6.x Docker Compose deployments. The tool checks collection load health for one collection or all collections in a database, enriches the result with Prometheus-backed Milvus runtime metrics, and exposes its own Prometheus metrics for alerting.

## Target Environment

- Milvus server: 2.6.22-compatible 2.6.x API
- Deployment: Docker Compose / standalone
- Milvus SDK endpoint: `localhost:19530` by default
- Prometheus endpoint: `http://localhost:9090` by default
- Milvus native metrics endpoint: scraped by Prometheus from Milvus `:9091/metrics`

## Selected Approach

Use **Milvus Go SDK + Prometheus**.

- Milvus Go SDK provides authoritative collection metadata, collection listing, load state, load progress, partition metadata, indexes, and statistics.
- Prometheus stores and queries Milvus native runtime metrics such as request rate, latency, failures, loaded entity counts, segment counts, and process resource metrics.
- The checker produces normalized results in table or JSON format.
- The exporter exposes custom metrics derived from SDK checks, rather than duplicating every native Milvus metric.

## Libraries

- `github.com/milvus-io/milvus/client/v2/milvusclient` for Milvus 2.6.x collection and load-state APIs
- `github.com/prometheus/client_golang/api` and `github.com/prometheus/client_golang/api/prometheus/v1` for Prometheus HTTP API queries
- `github.com/prometheus/client_golang/prometheus` and `github.com/prometheus/client_golang/prometheus/promhttp` for exporter metrics
- `github.com/spf13/cobra` for `check` and `serve` subcommands
- `github.com/stretchr/testify` for focused unit tests

## CLI Shape

The binary is named `milvus-check`.

One-shot check:

```powershell
milvus-check check --address localhost:19530 --database default --collection my_collection --prometheus-url http://localhost:9090 --output table
```

Check every collection in a database:

```powershell
milvus-check check --address localhost:19530 --database default --prometheus-url http://localhost:9090 --output json
```

Exporter:

```powershell
milvus-check serve --address milvus-standalone:19530 --database default --prometheus-url http://prometheus:9090 --listen :2112 --interval 30s
```

## Functional Requirements

1. Connect to Milvus using the configured address, username, password, token, and database.
2. If `--collection` is provided, inspect only that collection.
3. If `--collection` is omitted, list all collections in the configured database and inspect each one.
4. For each inspected collection, report:
   - existence
   - load state
   - load progress percent
   - entity count
   - index health summary
   - partition count
5. Query Prometheus for recent Milvus runtime indicators:
   - search/query request rate
   - search/query latency quantiles when histogram buckets are present
   - failed request rate
   - loaded entity count by collection when native labels are available
   - QueryNode segment count when collection ID labels are available
6. `check` outputs either a human-readable table or JSON.
7. `serve` periodically refreshes SDK-derived collection state and exposes custom Prometheus metrics.
8. Errors for one collection do not prevent checking the remaining collections.
9. The process exits non-zero when `check` finds a requested collection missing, an SDK connection failure, or a collection below the configured load threshold.

## Prometheus Metrics Strategy

Milvus native metrics include load-related signals, but they are not the authoritative source for exact collection load state. For exact load state and load progress, the tool uses the Milvus SDK. Prometheus is used for time-series context and runtime health.

The exporter exposes:

```text
milvus_check_up
milvus_check_collection_exists
milvus_check_collection_loaded
milvus_check_collection_load_progress_percent
milvus_check_collection_entities
milvus_check_collection_index_healthy
milvus_check_collection_check_errors_total
milvus_check_last_success_timestamp_seconds
```

Labels:

```text
database
collection
```

## Configuration

All runtime settings are loaded from a YAML file. The command line only accepts
`--config`, which defaults to `config.yaml`. This keeps Milvus, Prometheus,
logging, threshold, and exporter settings in one auditable place.

The configuration contains `milvus`, `prometheus`, `check`, `server`, and `log`
sections. Passwords and tokens are accepted by the loader but are never emitted
in logs or rendered output.

## Logging

The process uses structured logging written to stderr. Every operational log
contains a timestamp, level, message, and relevant fields such as command,
database, collection, duration, or error. Log level and JSON/text format are
configured in YAML. Startup, connection, collection checks, Prometheus query
degradation, refresh completion, and shutdown are logged explicitly.

## Architecture

The project is split into small packages with clear ownership:

- `cmd/milvus-check` owns process startup.
- `internal/cli` owns Cobra commands and flag parsing.
- `internal/config` owns validated runtime configuration.
- `internal/milvus` owns SDK interactions and converts Milvus API responses to internal types.
- `internal/promquery` owns Prometheus API queries and absent-metric handling.
- `internal/checker` combines SDK and Prometheus data into a report.
- `internal/output` renders table and JSON output.
- `internal/exporter` exposes custom Prometheus metrics and health endpoints.

## Error Handling

- SDK connection errors are fatal for the current command.
- Missing Prometheus URL disables Prometheus enrichment and keeps SDK checks working.
- Prometheus query failures are reported as warnings in `check` output and set exporter error metrics.
- Missing native Milvus metric series do not fail the check because metric names and labels can vary across Milvus patch versions and deployment shape.
- Collection-specific failures are captured in that collection's result and do not stop checks for other collections.

## Testing Strategy

- Unit tests cover config validation, collection target selection, health classification, Prometheus query construction, table rendering, JSON rendering, and exporter metric values.
- Milvus SDK and Prometheus clients are hidden behind interfaces so tests use fakes.
- Integration tests with real Docker Compose Milvus are documented but not required for ordinary unit test runs.

## Open Decisions

- The first implementation will use a compact table renderer from the standard library instead of adding a table formatting dependency.
- Prometheus metric names will be queried defensively. The tool will treat absent metric series as unavailable context instead of failure.
- Docker Compose files for Milvus and Prometheus can be added after the core binary works, because many users already have a Milvus Compose stack.
