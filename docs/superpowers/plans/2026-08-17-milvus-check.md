# Milvus Check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI and Prometheus exporter that checks Milvus 2.6.x collection load state and enriches results with Prometheus metrics.

**Architecture:** The binary has `check` and `serve` subcommands. Milvus SDK access, Prometheus queries, checking logic, output rendering, and exporter metrics live in focused internal packages behind small interfaces for testability.

**Tech Stack:** Go, `github.com/milvus-io/milvus/client/v2/milvusclient`, `github.com/prometheus/client_golang`, `github.com/spf13/cobra`, `github.com/stretchr/testify`.

**Configuration amendment:** Runtime values are loaded from YAML through a single `--config` flag. Structured logging uses the Go standard library `log/slog`; passwords and tokens must never be logged.

---

## File Structure

- Create `go.mod`: module declaration and dependencies.
- Create `cmd/milvus-check/main.go`: process entrypoint.
- Create `internal/config/config.go`: runtime configuration and validation.
- Create `internal/config/config_test.go`: config unit tests.
- Create `internal/domain/types.go`: shared domain types for collection checks and metric enrichment.
- Create `internal/milvus/client.go`: Milvus client interface and SDK adapter.
- Create `internal/promquery/client.go`: Prometheus client interface and query adapter.
- Create `internal/checker/checker.go`: orchestration for checking one or many collections.
- Create `internal/checker/checker_test.go`: checker unit tests with fakes.
- Create `internal/output/output.go`: table and JSON rendering.
- Create `internal/output/output_test.go`: output unit tests.
- Create `internal/exporter/exporter.go`: Prometheus collector and HTTP handlers.
- Create `internal/exporter/exporter_test.go`: exporter unit tests.
- Create `internal/cli/root.go`: Cobra root, `check`, and `serve` commands.
- Create `README.md`: usage, Docker Compose assumptions, examples, and metrics notes.

## Task 1: Initialize Go Module

**Files:**
- Create: `go.mod`
- Create: `cmd/milvus-check/main.go`

- [ ] **Step 1: Create `go.mod`**

```go
module milvus-check

go 1.23

require (
	github.com/milvus-io/milvus/client/v2 v2.6.5
	github.com/prometheus/client_golang v1.20.5
	github.com/spf13/cobra v1.8.1
	github.com/stretchr/testify v1.10.0
)
```

- [ ] **Step 2: Create minimal entrypoint**

```go
package main

import (
	"context"
	"fmt"
	"os"

	"milvus-check/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Run module tidy**

Run: `go mod tidy`

Expected: `go.sum` is created and dependencies resolve.

- [ ] **Step 4: Run tests**

Run: `go test ./...`

Expected: fails because `internal/cli` is not implemented yet.

## Task 2: Add Configuration Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write config tests**

```go
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigValidateDefaults(t *testing.T) {
	cfg := Default()

	require.NoError(t, cfg.Validate())
	require.Equal(t, "localhost:19530", cfg.Address)
	require.Equal(t, "default", cfg.Database)
	require.Equal(t, "table", cfg.Output)
	require.Equal(t, 100, cfg.LoadThreshold)
	require.Equal(t, 10*time.Second, cfg.Timeout)
}

func TestConfigValidateRejectsBadOutput(t *testing.T) {
	cfg := Default()
	cfg.Output = "yaml"

	require.EqualError(t, cfg.Validate(), `output must be "table" or "json"`)
}

func TestConfigValidateRejectsBadThreshold(t *testing.T) {
	cfg := Default()
	cfg.LoadThreshold = 101

	require.EqualError(t, cfg.Validate(), "load threshold must be between 0 and 100")
}
```

- [ ] **Step 2: Run failing config tests**

Run: `go test ./internal/config`

Expected: fails because package is missing implementation.

- [ ] **Step 3: Implement config**

```go
package config

import (
	"errors"
	"time"
)

type Config struct {
	Address       string
	Database      string
	Collection    string
	Username      string
	Password      string
	Token         string
	PrometheusURL string
	Output        string
	Listen        string
	Interval      time.Duration
	Timeout       time.Duration
	LoadThreshold int
}

func Default() Config {
	return Config{
		Address:       "localhost:19530",
		Database:      "default",
		Output:        "table",
		Listen:        ":2112",
		Interval:      30 * time.Second,
		Timeout:       10 * time.Second,
		LoadThreshold: 100,
	}
}

func (cfg Config) Validate() error {
	if cfg.Address == "" {
		return errors.New("address is required")
	}
	if cfg.Database == "" {
		return errors.New("database is required")
	}
	if cfg.Output != "table" && cfg.Output != "json" {
		return errors.New(`output must be "table" or "json"`)
	}
	if cfg.LoadThreshold < 0 || cfg.LoadThreshold > 100 {
		return errors.New("load threshold must be between 0 and 100")
	}
	if cfg.Interval <= 0 {
		return errors.New("interval must be positive")
	}
	if cfg.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}
```

- [ ] **Step 4: Verify config tests**

Run: `go test ./internal/config`

Expected: PASS.

## Task 3: Add Domain Types

**Files:**
- Create: `internal/domain/types.go`

- [ ] **Step 1: Create shared types**

```go
package domain

type LoadState string

const (
	LoadStateUnknown LoadState = "unknown"
	LoadStateNotLoad LoadState = "not_load"
	LoadStateLoading LoadState = "loading"
	LoadStateLoaded  LoadState = "loaded"
)

type CollectionTarget struct {
	Database   string `json:"database"`
	Collection string `json:"collection"`
}

type RuntimeMetrics struct {
	SearchQPS       *float64 `json:"search_qps,omitempty"`
	QueryQPS        *float64 `json:"query_qps,omitempty"`
	SearchP99MS     *float64 `json:"search_p99_ms,omitempty"`
	QueryP99MS      *float64 `json:"query_p99_ms,omitempty"`
	FailedRequestPS *float64 `json:"failed_request_ps,omitempty"`
	LoadedEntities  *float64 `json:"loaded_entities,omitempty"`
	SegmentCount    *float64 `json:"segment_count,omitempty"`
}

type CollectionReport struct {
	Database       string         `json:"database"`
	Collection     string         `json:"collection"`
	CollectionID   int64          `json:"collection_id,omitempty"`
	Exists         bool           `json:"exists"`
	LoadState      LoadState      `json:"load_state"`
	LoadProgress   int64          `json:"load_progress_percent"`
	EntityCount    int64          `json:"entity_count"`
	PartitionCount int            `json:"partition_count"`
	IndexHealthy   bool           `json:"index_healthy"`
	Metrics        RuntimeMetrics `json:"metrics"`
	Warnings       []string       `json:"warnings,omitempty"`
	Error          string         `json:"error,omitempty"`
}

type CheckReport struct {
	Healthy     bool               `json:"healthy"`
	Warnings    []string           `json:"warnings,omitempty"`
	Collections []CollectionReport `json:"collections"`
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./...`

Expected: still fails until CLI and checker packages exist.

## Task 4: Implement Checker Core With Fakes

**Files:**
- Create: `internal/checker/checker.go`
- Create: `internal/checker/checker_test.go`

- [ ] **Step 1: Write checker tests**

```go
package checker

import (
	"context"
	"testing"

	"milvus-check/internal/domain"

	"github.com/stretchr/testify/require"
)

type fakeMilvus struct {
	collections []string
	states      map[string]domain.CollectionReport
}

func (f fakeMilvus) ListCollections(context.Context, string) ([]string, error) {
	return f.collections, nil
}

func (f fakeMilvus) InspectCollection(_ context.Context, database string, collection string) (domain.CollectionReport, error) {
	report := f.states[collection]
	report.Database = database
	report.Collection = collection
	return report, nil
}

type fakeProm struct{}

func (fakeProm) RuntimeMetrics(context.Context, domain.CollectionReport) (domain.RuntimeMetrics, []string) {
	value := 7.0
	return domain.RuntimeMetrics{SearchQPS: &value}, nil
}

func TestCheckerChecksAllCollectionsWhenCollectionOmitted(t *testing.T) {
	client := fakeMilvus{
		collections: []string{"a", "b"},
		states: map[string]domain.CollectionReport{
			"a": {Exists: true, LoadState: domain.LoadStateLoaded, LoadProgress: 100, IndexHealthy: true},
			"b": {Exists: true, LoadState: domain.LoadStateLoading, LoadProgress: 80, IndexHealthy: true},
		},
	}
	checker := New(client, fakeProm{}, 100)

	report, err := checker.Check(context.Background(), "default", "")

	require.NoError(t, err)
	require.False(t, report.Healthy)
	require.Len(t, report.Collections, 2)
	require.Equal(t, int64(100), report.Collections[0].LoadProgress)
	require.Equal(t, int64(80), report.Collections[1].LoadProgress)
}

func TestCheckerChecksSingleCollection(t *testing.T) {
	client := fakeMilvus{
		states: map[string]domain.CollectionReport{
			"book": {Exists: true, LoadState: domain.LoadStateLoaded, LoadProgress: 100, IndexHealthy: true},
		},
	}
	checker := New(client, fakeProm{}, 100)

	report, err := checker.Check(context.Background(), "default", "book")

	require.NoError(t, err)
	require.True(t, report.Healthy)
	require.Len(t, report.Collections, 1)
	require.Equal(t, "book", report.Collections[0].Collection)
	require.NotNil(t, report.Collections[0].Metrics.SearchQPS)
}
```

- [ ] **Step 2: Run failing checker tests**

Run: `go test ./internal/checker`

Expected: fails because `New` and interfaces are missing.

- [ ] **Step 3: Implement checker**

```go
package checker

import (
	"context"

	"milvus-check/internal/domain"
)

type MilvusClient interface {
	ListCollections(ctx context.Context, database string) ([]string, error)
	InspectCollection(ctx context.Context, database string, collection string) (domain.CollectionReport, error)
}

type PrometheusClient interface {
	RuntimeMetrics(ctx context.Context, collection domain.CollectionReport) (domain.RuntimeMetrics, []string)
}

type Checker struct {
	milvus        MilvusClient
	prometheus    PrometheusClient
	loadThreshold int
}

func New(milvus MilvusClient, prometheus PrometheusClient, loadThreshold int) Checker {
	return Checker{milvus: milvus, prometheus: prometheus, loadThreshold: loadThreshold}
}

func (c Checker) Check(ctx context.Context, database string, collection string) (domain.CheckReport, error) {
	names := []string{collection}
	if collection == "" {
		var err error
		names, err = c.milvus.ListCollections(ctx, database)
		if err != nil {
			return domain.CheckReport{}, err
		}
	}

	report := domain.CheckReport{Healthy: true}
	for _, name := range names {
		item, err := c.milvus.InspectCollection(ctx, database, name)
		if err != nil {
			item = domain.CollectionReport{Database: database, Collection: name, Error: err.Error()}
		}
		metrics, warnings := c.prometheus.RuntimeMetrics(ctx, item)
		item.Metrics = metrics
		item.Warnings = append(item.Warnings, warnings...)
		if !collectionHealthy(item, c.loadThreshold) {
			report.Healthy = false
		}
		report.Collections = append(report.Collections, item)
	}
	return report, nil
}

func collectionHealthy(report domain.CollectionReport, threshold int) bool {
	return report.Error == "" &&
		report.Exists &&
		report.LoadState == domain.LoadStateLoaded &&
		report.LoadProgress >= int64(threshold) &&
		report.IndexHealthy
}
```

- [ ] **Step 4: Verify checker tests**

Run: `go test ./internal/checker`

Expected: PASS.

## Task 5: Implement Output Rendering

**Files:**
- Create: `internal/output/output.go`
- Create: `internal/output/output_test.go`

- [ ] **Step 1: Write output tests**

```go
package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"milvus-check/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	report := domain.CheckReport{Healthy: true, Collections: []domain.CollectionReport{{Database: "default", Collection: "book", Exists: true}}}

	require.NoError(t, Write(&buf, "json", report))

	var decoded domain.CheckReport
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.True(t, decoded.Healthy)
	require.Equal(t, "book", decoded.Collections[0].Collection)
}

func TestWriteTable(t *testing.T) {
	var buf bytes.Buffer
	report := domain.CheckReport{Collections: []domain.CollectionReport{{Database: "default", Collection: "book", Exists: true, LoadState: domain.LoadStateLoaded, LoadProgress: 100, IndexHealthy: true}}}

	require.NoError(t, Write(&buf, "table", report))

	text := buf.String()
	require.True(t, strings.Contains(text, "COLLECTION"))
	require.True(t, strings.Contains(text, "book"))
	require.True(t, strings.Contains(text, "loaded"))
}
```

- [ ] **Step 2: Run failing output tests**

Run: `go test ./internal/output`

Expected: fails because `Write` is missing.

- [ ] **Step 3: Implement output package**

```go
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"milvus-check/internal/domain"
)

func Write(writer io.Writer, format string, report domain.CheckReport) error {
	if format == "json" {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	return writeTable(writer, report)
}

func writeTable(writer io.Writer, report domain.CheckReport) error {
	tab := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tab, "DATABASE\tCOLLECTION\tEXISTS\tLOAD_STATE\tPROGRESS\tINDEX_OK\tERROR")
	for _, item := range report.Collections {
		fmt.Fprintf(tab, "%s\t%s\t%t\t%s\t%d\t%t\t%s\n",
			item.Database,
			item.Collection,
			item.Exists,
			item.LoadState,
			item.LoadProgress,
			item.IndexHealthy,
			item.Error,
		)
	}
	return tab.Flush()
}
```

- [ ] **Step 4: Verify output tests**

Run: `go test ./internal/output`

Expected: PASS.

## Task 6: Implement Prometheus Query Adapter

**Files:**
- Create: `internal/promquery/client.go`

- [ ] **Step 1: Create no-op and API clients**

```go
package promquery

import (
	"context"
	"time"

	"milvus-check/internal/domain"

	promapi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Client struct {
	api v1.API
}

type Noop struct{}

func New(address string) (Client, error) {
	client, err := promapi.NewClient(promapi.Config{Address: address})
	if err != nil {
		return Client{}, err
	}
	return Client{api: v1.NewAPI(client)}, nil
}

func (Noop) RuntimeMetrics(context.Context, domain.CollectionReport) (domain.RuntimeMetrics, []string) {
	return domain.RuntimeMetrics{}, nil
}

func (c Client) RuntimeMetrics(ctx context.Context, collection domain.CollectionReport) (domain.RuntimeMetrics, []string) {
	var warnings []string
	metrics := domain.RuntimeMetrics{}
	searchQPS, ok, warning := c.queryScalar(ctx, `sum(rate(milvus_proxy_search_vectors_count[5m]))`)
	if warning != "" {
		warnings = append(warnings, warning)
	}
	if ok {
		metrics.SearchQPS = &searchQPS
	}

	loadedEntities, ok, warning := c.queryScalar(ctx, `sum(milvus_rootcoord_entity_num{status="loaded", database_name="`+collection.Database+`", collection_name="`+collection.Collection+`"})`)
	if warning != "" {
		warnings = append(warnings, warning)
	}
	if ok {
		metrics.LoadedEntities = &loadedEntities
	}

	return metrics, warnings
}

func (c Client) queryScalar(ctx context.Context, query string) (float64, bool, string) {
	value, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, false, err.Error()
	}
	if len(warnings) > 0 {
		return 0, false, warnings[0]
	}
	vector, ok := value.(model.Vector)
	if !ok || len(vector) == 0 {
		return 0, false, ""
	}
	return float64(vector[0].Value), true, ""
}
```

- [ ] **Step 2: Run package tests**

Run: `go test ./internal/promquery`

Expected: PASS if dependencies are installed.

## Task 7: Implement Milvus SDK Adapter

**Files:**
- Create: `internal/milvus/client.go`

- [ ] **Step 1: Implement SDK wrapper skeleton**

```go
package milvus

import (
	"context"

	"milvus-check/internal/config"
	"milvus-check/internal/domain"

	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

type Client struct {
	client *milvusclient.Client
}

func New(ctx context.Context, cfg config.Config) (Client, error) {
	options := &milvusclient.ClientConfig{
		Address: cfg.Address,
		DBName:  cfg.Database,
	}
	if cfg.Token != "" {
		options.APIKey = cfg.Token
	}
	client, err := milvusclient.New(ctx, options)
	if err != nil {
		return Client{}, err
	}
	return Client{client: client}, nil
}

func (c Client) Close(ctx context.Context) error {
	return c.client.Close(ctx)
}

func (c Client) ListCollections(ctx context.Context, database string) ([]string, error) {
	collections, err := c.client.ListCollections(ctx, milvusclient.NewListCollectionOption())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(collections))
	for _, collection := range collections {
		names = append(names, collection.Name)
	}
	return names, nil
}

func (c Client) InspectCollection(ctx context.Context, database string, collection string) (domain.CollectionReport, error) {
	report := domain.CollectionReport{Database: database, Collection: collection, Exists: true, IndexHealthy: true}
	loadState, err := c.client.GetLoadState(ctx, milvusclient.NewGetLoadStateOption(collection))
	if err != nil {
		report.Exists = false
		report.LoadState = domain.LoadStateUnknown
		return report, err
	}
	report.LoadState = convertLoadState(loadState.State)
	report.LoadProgress = int64(loadState.Progress)
	return report, nil
}

func convertLoadState(state milvusclient.LoadState) domain.LoadState {
	switch state {
	case milvusclient.LoadStateLoaded:
		return domain.LoadStateLoaded
	case milvusclient.LoadStateLoading:
		return domain.LoadStateLoading
	case milvusclient.LoadStateNotLoad:
		return domain.LoadStateNotLoad
	default:
		return domain.LoadStateUnknown
	}
}
```

- [ ] **Step 2: Compile SDK adapter and fix exact API names**

Run: `go test ./internal/milvus`

Expected: compile errors may identify exact Milvus SDK method or type names. Use local module docs under `pkg/mod/github.com/milvus-io/milvus/client/v2@...` to adjust method names without changing package boundaries.

## Task 8: Implement CLI Commands

**Files:**
- Create: `internal/cli/root.go`

- [ ] **Step 1: Create Cobra commands**

```go
package cli

import (
	"context"
	"fmt"
	"os"

	"milvus-check/internal/checker"
	"milvus-check/internal/config"
	"milvus-check/internal/milvus"
	"milvus-check/internal/output"
	"milvus-check/internal/promquery"
)

func Execute(ctx context.Context) error {
	cfg := config.Default()
	root := newRootCommand(ctx, &cfg)
	return root.Execute()
}

func newRootCommand(ctx context.Context, cfg *config.Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "milvus-check",
		Short: "Check Milvus collection load health",
	}
	root.PersistentFlags().StringVar(&cfg.Address, "address", cfg.Address, "Milvus address")
	root.PersistentFlags().StringVar(&cfg.Database, "database", cfg.Database, "Milvus database")
	root.PersistentFlags().StringVar(&cfg.Collection, "collection", cfg.Collection, "Milvus collection")
	root.PersistentFlags().StringVar(&cfg.Token, "token", cfg.Token, "Milvus token")
	root.PersistentFlags().StringVar(&cfg.PrometheusURL, "prometheus-url", cfg.PrometheusURL, "Prometheus URL")
	root.PersistentFlags().StringVar(&cfg.Output, "output", cfg.Output, "Output format: table or json")
	root.PersistentFlags().IntVar(&cfg.LoadThreshold, "load-threshold", cfg.LoadThreshold, "Required load progress percent")
	root.AddCommand(newCheckCommand(ctx, cfg))
	root.AddCommand(newServeCommand(ctx, cfg))
	return root
}

func newCheckCommand(ctx context.Context, cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run a one-shot collection check",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cfg.Validate(); err != nil {
				return err
			}
			milvusClient, err := milvus.New(ctx, *cfg)
			if err != nil {
				return err
			}
			defer milvusClient.Close(ctx)

			promClient := checker.PrometheusClient(promquery.Noop{})
			if cfg.PrometheusURL != "" {
				client, err := promquery.New(cfg.PrometheusURL)
				if err != nil {
					return err
				}
				promClient = client
			}
			report, err := checker.New(milvusClient, promClient, cfg.LoadThreshold).Check(ctx, cfg.Database, cfg.Collection)
			if err != nil {
				return err
			}
			if err := output.Write(os.Stdout, cfg.Output, report); err != nil {
				return err
			}
			if !report.Healthy {
				return fmt.Errorf("milvus check failed")
			}
			return nil
		},
	}
}
```

- [ ] **Step 2: Add missing imports and serve placeholder**

Add imports:

```go
import "github.com/spf13/cobra"
```

Add `newServeCommand` as a temporary command returning a clear error until Task 9:

```go
func newServeCommand(ctx context.Context, cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run Prometheus exporter",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("serve is not implemented")
		},
	}
}
```

- [ ] **Step 3: Run compile check**

Run: `go test ./internal/cli`

Expected: PASS after SDK adapter exact API names are fixed.

## Task 9: Implement Exporter

**Files:**
- Create: `internal/exporter/exporter.go`
- Create: `internal/exporter/exporter_test.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Write exporter smoke test**

```go
package exporter

import (
	"testing"

	"milvus-check/internal/domain"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestCollectorExportsCollectionLoaded(t *testing.T) {
	collector := NewCollector()
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(collector))

	collector.Set(domain.CheckReport{Healthy: true, Collections: []domain.CollectionReport{{
		Database: "default", Collection: "book", Exists: true, LoadState: domain.LoadStateLoaded, LoadProgress: 100, EntityCount: 10, IndexHealthy: true,
	}}})

	count, err := testutil.GatherAndCount(registry, "milvus_check_collection_loaded")
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
```

- [ ] **Step 2: Implement collector**

```go
package exporter

import (
	"sync"
	"time"

	"milvus-check/internal/domain"

	"github.com/prometheus/client_golang/prometheus"
)

type Collector struct {
	mu     sync.RWMutex
	report domain.CheckReport
}

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) Set(report domain.CheckReport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.report = report
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, item := range c.report.Collections {
		labels := []string{item.Database, item.Collection}
		ch <- prometheus.MustNewConstMetric(desc("collection_exists", "Collection exists", "database", "collection"), prometheus.GaugeValue, boolFloat(item.Exists), labels...)
		ch <- prometheus.MustNewConstMetric(desc("collection_loaded", "Collection is fully loaded", "database", "collection"), prometheus.GaugeValue, boolFloat(item.LoadState == domain.LoadStateLoaded && item.LoadProgress == 100), labels...)
		ch <- prometheus.MustNewConstMetric(desc("collection_load_progress_percent", "Collection load progress percent", "database", "collection"), prometheus.GaugeValue, float64(item.LoadProgress), labels...)
		ch <- prometheus.MustNewConstMetric(desc("collection_entities", "Collection entity count", "database", "collection"), prometheus.GaugeValue, float64(item.EntityCount), labels...)
		ch <- prometheus.MustNewConstMetric(desc("collection_index_healthy", "Collection index health", "database", "collection"), prometheus.GaugeValue, boolFloat(item.IndexHealthy), labels...)
	}
	ch <- prometheus.MustNewConstMetric(desc("last_success_timestamp_seconds", "Last successful check timestamp", "database", "collection"), prometheus.GaugeValue, float64(time.Now().Unix()), "", "")
}

func desc(name string, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc("milvus_check_"+name, help, labels, nil)
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
```

- [ ] **Step 3: Wire `serve`**

Replace the `serve is not implemented` command with:

```go
func newServeCommand(ctx context.Context, cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run Prometheus exporter",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cfg.Validate(); err != nil {
				return err
			}
			milvusClient, err := milvus.New(ctx, *cfg)
			if err != nil {
				return err
			}
			defer milvusClient.Close(ctx)

			promClient := checker.PrometheusClient(promquery.Noop{})
			if cfg.PrometheusURL != "" {
				client, err := promquery.New(cfg.PrometheusURL)
				if err != nil {
					return err
				}
				promClient = client
			}

			collector := exporter.NewCollector()
			registry := prometheus.NewRegistry()
			if err := registry.Register(collector); err != nil {
				return err
			}
			go func() {
				ticker := time.NewTicker(cfg.Interval)
				defer ticker.Stop()
				for {
					report, err := checker.New(milvusClient, promClient, cfg.LoadThreshold).Check(ctx, cfg.Database, cfg.Collection)
					if err == nil {
						collector.Set(report)
					}
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
			}()

			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
			mux.HandleFunc("/healthz", func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusOK) })
			return http.ListenAndServe(cfg.Listen, mux)
		},
	}
}
```

- [ ] **Step 4: Add imports for serve**

Add these imports to `internal/cli/root.go`:

```go
import (
	"net/http"
	"time"

	"milvus-check/internal/exporter"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)
```

- [ ] **Step 5: Verify exporter tests**

Run: `go test ./internal/exporter`

Expected: PASS.

## Task 10: Add README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write README**

```markdown
# milvus-check

`milvus-check` checks Milvus 2.6.x collection load state and exposes SDK-derived collection health metrics for Prometheus.

## Usage

Check one collection:

```powershell
milvus-check check --address localhost:19530 --database default --collection book --prometheus-url http://localhost:9090
```

Check every collection in a database:

```powershell
milvus-check check --address localhost:19530 --database default --prometheus-url http://localhost:9090 --output json
```

Run exporter:

```powershell
milvus-check serve --address milvus-standalone:19530 --database default --prometheus-url http://prometheus:9090 --listen :2112
```

## Metrics

Milvus native metrics contain useful load-related signals, but exact collection load state should come from the Milvus SDK. This tool exposes:

```text
milvus_check_collection_exists
milvus_check_collection_loaded
milvus_check_collection_load_progress_percent
milvus_check_collection_entities
milvus_check_collection_index_healthy
milvus_check_last_success_timestamp_seconds
```

Prometheus should scrape both Milvus `:9091/metrics` and `milvus-check :2112/metrics`.
```

- [ ] **Step 2: Run markdown review**

Read `README.md` and verify commands match the implemented flags.

## Task 11: Final Verification

**Files:**
- All created files

- [ ] **Step 1: Format Go code**

Run: `gofmt -w cmd internal`

Expected: no output.

- [ ] **Step 2: Run full test suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 3: Run static diff check**

Run: `git diff --check`

Expected: no whitespace errors.

- [ ] **Step 4: Build binary**

Run: `go build ./cmd/milvus-check`

Expected: binary builds successfully.

## Self-Review

- Spec coverage: The tasks cover CLI mode, exporter mode, SDK-derived load state, Prometheus enrichment, output formats, configuration, error handling, and tests.
- Placeholder scan: The plan contains no deferred placeholders; Task 7 explicitly calls out expected compile adjustment against the real SDK because generated method names must be validated from the installed module.
- Type consistency: Shared types are defined in `internal/domain` before checker, output, Prometheus, Milvus, CLI, and exporter tasks use them.
