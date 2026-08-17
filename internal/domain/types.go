package domain

import "time"

type LoadState string

const (
	LoadStateUnknown LoadState = "unknown"
	LoadStateNotLoad LoadState = "not_load"
	LoadStateLoading LoadState = "loading"
	LoadStateLoaded  LoadState = "loaded"
)

type RuntimeMetrics struct {
	SearchQPS       *float64 `json:"search_qps,omitempty"`
	QueryQPS        *float64 `json:"query_qps,omitempty"`
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
	CheckedAt   time.Time          `json:"checked_at"`
	Warnings    []string           `json:"warnings,omitempty"`
	Collections []CollectionReport `json:"collections"`
}
