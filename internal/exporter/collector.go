package exporter

import (
	"sync"
	"time"

	"milvus-check/internal/dashboard"
	"milvus-check/internal/domain"

	"github.com/prometheus/client_golang/prometheus"
)

type Collector struct {
	mu           sync.Mutex
	store        *dashboard.Store
	lastObserved time.Time
	errorCounts  map[string]uint64
	desc         descriptions
}

type descriptions struct {
	up              *prometheus.Desc
	exists          *prometheus.Desc
	loaded          *prometheus.Desc
	progress        *prometheus.Desc
	entities        *prometheus.Desc
	indexHealthy    *prometheus.Desc
	checkErrors     *prometheus.Desc
	lastSuccessTime *prometheus.Desc
}

func NewCollector(store *dashboard.Store) *Collector {
	labels := []string{"database", "collection"}
	return &Collector{
		store:       store,
		errorCounts: make(map[string]uint64),
		desc: descriptions{
			up:              prometheus.NewDesc("milvus_check_up", "Milvus SDK check can run successfully.", nil, nil),
			exists:          prometheus.NewDesc("milvus_check_collection_exists", "Whether the collection exists.", labels, nil),
			loaded:          prometheus.NewDesc("milvus_check_collection_loaded", "Whether the collection is fully loaded.", labels, nil),
			progress:        prometheus.NewDesc("milvus_check_collection_load_progress_percent", "Collection load progress percent.", labels, nil),
			entities:        prometheus.NewDesc("milvus_check_collection_entities", "Collection entity count reported by Milvus.", labels, nil),
			indexHealthy:    prometheus.NewDesc("milvus_check_collection_index_healthy", "Whether all reported indexes are healthy.", labels, nil),
			checkErrors:     prometheus.NewDesc("milvus_check_collection_check_errors_total", "Collection check errors observed by the exporter.", labels, nil),
			lastSuccessTime: prometheus.NewDesc("milvus_check_last_success_timestamp_seconds", "Unix timestamp of the latest completed SDK check.", nil, nil),
		},
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc.up
	ch <- c.desc.exists
	ch <- c.desc.loaded
	ch <- c.desc.progress
	ch <- c.desc.entities
	ch <- c.desc.indexHealthy
	ch <- c.desc.checkErrors
	ch <- c.desc.lastSuccessTime
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	snapshot := c.store.Snapshot()
	c.mu.Lock()
	defer c.mu.Unlock()
	if snapshot.Ready && snapshot.Report.CheckedAt.After(c.lastObserved) {
		for _, item := range snapshot.Report.Collections {
			if item.Error != "" {
				c.errorCounts[item.Database+"\x00"+item.Collection]++
			}
		}
		c.lastObserved = snapshot.Report.CheckedAt
	}

	ch <- prometheus.MustNewConstMetric(c.desc.up, prometheus.GaugeValue, boolFloat(snapshot.Up))
	if snapshot.Ready {
		checkedAt := snapshot.Report.CheckedAt
		if checkedAt.IsZero() {
			checkedAt = time.Now()
		}
		ch <- prometheus.MustNewConstMetric(c.desc.lastSuccessTime, prometheus.GaugeValue, float64(checkedAt.Unix()))
	}
	for _, item := range snapshot.Report.Collections {
		labels := []string{item.Database, item.Collection}
		ch <- prometheus.MustNewConstMetric(c.desc.exists, prometheus.GaugeValue, boolFloat(item.Exists), labels...)
		ch <- prometheus.MustNewConstMetric(c.desc.loaded, prometheus.GaugeValue, boolFloat(item.LoadState == domain.LoadStateLoaded && item.LoadProgress == 100), labels...)
		ch <- prometheus.MustNewConstMetric(c.desc.progress, prometheus.GaugeValue, float64(item.LoadProgress), labels...)
		ch <- prometheus.MustNewConstMetric(c.desc.entities, prometheus.GaugeValue, float64(item.EntityCount), labels...)
		ch <- prometheus.MustNewConstMetric(c.desc.indexHealthy, prometheus.GaugeValue, boolFloat(item.IndexHealthy), labels...)
		ch <- prometheus.MustNewConstMetric(c.desc.checkErrors, prometheus.CounterValue, float64(c.errorCounts[item.Database+"\x00"+item.Collection]), labels...)
	}
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
