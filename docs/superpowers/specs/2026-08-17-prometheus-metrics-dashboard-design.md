# Prometheus Metrics Dashboard Design

## Goal

Extend the embedded `milvus-check` web UI into a Milvus-specific Prometheus dashboard with accurate Chinese names, explanations, units, compatibility reporting, current values, and selectable `5m`, `1h`, `6h`, and `24h` trends.

The dashboard must fit the metrics actually scraped from Milvus instead of relying on a Grafana template whose metric names and labels may not match the deployed version.

## Observed Environment

The configured Prometheus server is reachable at `http://10.40.0.184:9091`. Its current Milvus scrape target reports:

- `job="milvus"`
- `instance="10.54.56.88:9091"`
- `milvus_build_info{version="2.5.10", git_commit="5a8c98a"}`

The Milvus SDK endpoint is on the same host. The target will later move to Milvus 2.6.22, so the implementation must discover metric availability and tolerate metric or label differences between 2.5 and 2.6.

## Selected Approach

Use a curated server-side metric catalog and embedded time-series charts.

- The Go service owns all PromQL and Chinese metric metadata.
- The browser requests metric IDs and allowed time ranges, never arbitrary PromQL.
- The service discovers available metric names from Prometheus and selects a compatible query variant.
- Current values use Prometheus instant queries.
- Trends use Prometheus range queries with bounded range and step values.
- The page embeds a vendored uPlot runtime and does not use a CDN or separate frontend build.
- Existing collection SDK checks remain the authoritative source for exact collection load state.

## Metric Definition Model

Each catalog entry contains:

```go
type Definition struct {
    ID             string
    Category       string
    TitleZH        string
    DescriptionZH  string
    Interpretation string
    Source         string
    Unit           Unit
    Visualization  Visualization
    Queries        []QueryVariant
    Thresholds     *Thresholds
}

type QueryVariant struct {
    RequiredMetrics []string
    InstantPromQL   string
    RangePromQL     string
}
```

`QueryVariant` entries are ordered. The first variant whose required metrics are present in Prometheus is selected. The selected variant and rendered PromQL are included in API output for verification.

## Availability Semantics

The API and page distinguish these states:

- `available`: the required metric exists and the query returned data.
- `zero`: the query returned a real numeric zero.
- `no_data`: the metric exists but the selected time window has no samples.
- `unsupported`: no compatible required metric exists for the current Milvus target.
- `error`: Prometheus rejected or timed out the query.

Unsupported and no-data metrics display a Chinese explanation and never silently appear as zero.

## Prometheus Discovery

The service caches the following discovery data:

- metric names from `/api/v1/label/__name__/values`
- Milvus version and build commit from `milvus_build_info`
- runtime metadata from `milvus_runtime_info`
- configured `job` selector, defaulting to `milvus`

Discovery refreshes every five minutes and immediately after a previously unsupported metric becomes available through a regular refresh cycle.

The YAML configuration adds:

```yaml
prometheus:
  enabled: true
  address: "http://localhost:9090"
  timeout: 10s
  job: "milvus"

metrics:
  default_range: "1h"
  query_timeout: 15s
  thresholds: {}
```

The Prometheus address and selectors are never returned to the browser.

## Time Ranges

Only four ranges are accepted:

| Range | Step | Rate Window |
|---|---:|---:|
| `5m` | 15 seconds | 1 minute |
| `1h` | 30 seconds | 5 minutes |
| `6h` | 2 minutes | 10 minutes |
| `24h` | 5 minutes | 15 minutes |

The backend calculates start, end, step, and rate window. The browser cannot override them.

## Initial Metric Catalog

### Overview

| ID | Chinese name | Primary source | Unit |
|---|---|---|---|
| `milvus_up` | Milvus 抓取状态 | `up{job="milvus"}` | boolean |
| `request_qps` | 总请求速率 | `milvus_proxy_req_count{status="total"}` | requests/s |
| `search_qps` | 向量搜索速率 | `function_name="Search"` | requests/s |
| `query_qps` | 标量查询速率 | `function_name="Query"` | requests/s |
| `insert_qps` | 写入请求速率 | `function_name="Insert"` | requests/s |
| `request_success_rate` | 请求成功率 | success / total request counters | percent |
| `request_p95` | 请求 P95 延迟 | `milvus_proxy_req_latency_bucket` | ms |
| `request_p99` | 请求 P99 延迟 | `milvus_proxy_req_latency_bucket` | ms |
| `proxy_tt_lag` | Proxy 时间同步延迟 | `milvus_proxy_tt_lag_ms` | ms |

### Request And Latency

Request panels aggregate and group by `function_name`:

- request rate by operation
- success rate by operation
- P95 latency by operation
- P99 latency by operation
- rate-limited requests
- request queue wait P95
- received and sent traffic rate

Known operation labels receive Chinese mappings. Unknown operations retain the original label and show `暂无中文映射`.

### QueryNode

- Search/Query request P95 and P99 from `milvus_querynode_sq_req_latency_bucket`
- queue latency P95 from `milvus_querynode_sq_queue_latency_bucket`
- core processing latency P95 from `milvus_querynode_sq_core_latency_bucket`
- ready task count from `milvus_querynode_read_task_ready_len`
- unsolved task count from `milvus_querynode_read_task_unsolved_len`
- read concurrency from `milvus_querynode_read_task_concurrency`
- message dispatcher and DML channel time lag
- QueryNode memory high-water signal as informational data until its deployment-specific meaning is configured

### Write And Storage

- inserted vector rate from `milvus_proxy_insert_vectors_count`
- DataNode consumed messages and bytes rate
- DataNode write rows rate
- flush request success/failure rate
- flush save P95 latency
- foreground buffer size
- Segment count grouped by state and level
- stored rows grouped by database, collection, and Segment state
- stored Binlog bytes
- channel checkpoint age and DataNode time lag

### Load And Index

- collection load request success rate
- full collection load P95/P99 latency
- QueryNode Segment load P95 latency
- QueryNode index load P95 latency
- Segment load concurrency
- index request success/failure rate
- index task count by state
- index task queue P95 latency
- index build and save P95 latency

The SDK collection table remains above this section and provides exact `loaded`, `loading`, and `not_load` states.

### Components

- Milvus version, Git commit, build time, metadata backend, and message queue backend
- component node count grouped by `role_name`
- collection, partition, DataNode, QueryNode, and IndexNode counts
- RootCoord and DataCoord operation error rates
- metadata request P95 latency
- storage request P95 latency
- Go process CPU rate, resident memory, goroutine count, and process uptime when standard process metrics are scraped

## PromQL Rules

PromQL templates use a server-side selector such as `{job="milvus"}`.

Counter rate:

```promql
sum(rate(milvus_proxy_req_count{job="milvus",status="total"}[5m]))
```

Success rate:

```promql
100 *
sum(rate(milvus_proxy_req_count{job="milvus",status="success"}[5m]))
/
clamp_min(sum(rate(milvus_proxy_req_count{job="milvus",status="total"}[5m])), 1e-9)
```

P95 latency in milliseconds:

```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(milvus_proxy_req_latency_bucket{job="milvus"}[5m]))
)
```

Collection labels use separate variants for `db_name` and `database_name` where required.

## API Contract

### Catalog

`GET /api/metrics/catalog`

Returns Milvus build information, categories, definitions, selected query variant, and compatibility state.

### Summary

`GET /api/metrics/summary?range=1h`

Returns current values for all overview metrics and category health counts. Queries execute with bounded concurrency and return partial results when one query fails.

### Series

`GET /api/metrics/series/{metric_id}?range=1h`

Returns:

```json
{
  "id": "request_p95",
  "title": "请求 P95 延迟",
  "description": "95% 的 Milvus 请求在该时间内完成",
  "unit": "ms",
  "state": "available",
  "current": 12.6,
  "level": "normal",
  "promql": "histogram_quantile(...) ",
  "series": [
    {
      "name": "全部请求",
      "labels": {},
      "points": [[1786915200, 11.8]]
    }
  ]
}
```

Unknown IDs and ranges return HTTP 400. Prometheus timeout returns a structured metric error without exposing the Prometheus URL.

## Page Layout

The existing collection load view remains the first operational section.

The page adds a sticky category navigation:

- 总览
- 请求与延迟
- 查询节点
- 写入与存储
- 加载与索引
- 组件状态
- 指标字典

The header adds the allowed time-range segmented control. Changing range reloads visible trend panels and preserves the collection snapshot.

Each metric panel contains:

- Chinese title
- current value and unit
- availability and threshold state
- line chart for continuous time series or horizontal bars for state/category comparisons
- legend with text and line-style distinction
- information button opening a metric-detail drawer

The detail drawer contains Chinese meaning, interpretation, source component, original metric names, label meanings, actual PromQL, compatibility state, and threshold source.

The metric dictionary provides searchable rows by Chinese title, original metric, component, and category.

Desktop uses a two-column chart grid. Tablet and mobile use one column. Charts have stable heights and never resize the page during loading. Missing data uses a framed empty state instead of an empty canvas.

## Chart Runtime

Vendor minified uPlot JavaScript and CSS under `internal/dashboard/assets/vendor/`. Go embeds the files into the existing binary. No CDN, npm runtime, or frontend build is required.

Continuous time series use line charts. More than six series are reduced to the largest contributors plus `其他`. Segment and component state comparisons use horizontal bars with visible numeric labels.

Charts support hover values and a text legend. They do not use decorative animation. `prefers-reduced-motion` is respected.

## Thresholds

Defaults apply only to unambiguous health signals:

| Metric | Normal | Warning | Critical |
|---|---:|---:|---:|
| request success rate | `>= 99%` | `95% - 99%` | `< 95%` |
| Proxy time lag | `< 5000ms` | `5000 - 30000ms` | `> 30000ms` |
| DataNode consume lag | `< 5000ms` | `5000 - 30000ms` | `> 30000ms` |
| scrape/check up | `1` | - | `0` |
| collection load progress | `100%` | `1% - 99%` | `0%` |

Latency, queue length, Segment count, storage size, load duration, and memory high-water values remain informational by default. YAML overrides are keyed by metric ID and identify whether higher or lower values are worse.

## Error Handling

- Prometheus disabled: the collection dashboard remains available and the metric area displays a configuration-level unavailable state.
- Discovery failure: retain the previous catalog and mark it stale.
- One metric query failure: render that panel as error and continue other queries.
- Unsupported metric: show current version and missing required metric names.
- Range query timeout: show a retry action for that panel.
- Browser requests are cancelled when the user switches range before the previous response completes.

## Security And Performance

- The browser cannot submit arbitrary PromQL.
- Prometheus URL and credentials are not returned by any API.
- Metric IDs and ranges are allowlisted.
- Instant queries use bounded concurrency.
- Range results are cached by metric ID, range, and selected variant for one refresh interval.
- API responses cap series count and point count.
- Metric explanations and labels are rendered with DOM text APIs.

## Testing

- Catalog tests cover Chinese metadata, query selection, 2.5/2.6 alternatives, thresholds, and operation-name mapping.
- Prometheus adapter tests cover discovery, instant query, range query, no-data, timeout, and partial failure.
- HTTP tests cover allowlisted ranges/IDs, response contracts, disabled Prometheus, and no credential leakage.
- Frontend tests cover range switching, metric state rendering, dictionary filtering, and absent data.
- Browser verification covers desktop, tablet, and mobile layouts with populated, empty, unsupported, stale, and error metrics.
- Final verification runs `go test ./...`, `go vet ./...`, build, and `git diff --check`.

## Scope Exclusions

- No arbitrary PromQL editor.
- No alert rule creation or Alertmanager integration.
- No long-term local metric storage.
- No replacement for Prometheus retention or Grafana ad hoc analysis.
- No Milvus write or administrative actions.
