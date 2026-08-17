package dashboard

import (
	"sync"
	"time"

	"milvus-check/internal/domain"
)

// Snapshot 是页面、readiness 和 exporter 共享的只读状态快照。
type Snapshot struct {
	Ready                  bool
	Up                     bool
	Report                 domain.CheckReport
	RefreshIntervalSeconds int64
	LastError              string
}

// Store 串行化后台刷新与多个 HTTP 读取之间的状态访问。
type Store struct {
	mu                     sync.RWMutex
	ready                  bool
	up                     bool
	report                 domain.CheckReport
	refreshIntervalSeconds int64
	lastError              string
}

func NewStore(interval time.Duration) *Store {
	seconds := int64(interval / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return &Store{refreshIntervalSeconds: seconds}
}

func (s *Store) SetSuccess(report domain.CheckReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
	s.up = true
	s.report = cloneReport(report)
	s.lastError = ""
}

// SetFailure 保留最后一次成功数据，仅更新当前采集链路状态。
func (s *Store) SetFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.up = false
	if err != nil {
		s.lastError = err.Error()
	}
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		Ready:                  s.ready,
		Up:                     s.up,
		Report:                 cloneReport(s.report),
		RefreshIntervalSeconds: s.refreshIntervalSeconds,
		LastError:              s.lastError,
	}
}

func cloneReport(report domain.CheckReport) domain.CheckReport {
	cloned := report
	cloned.Warnings = append([]string(nil), report.Warnings...)
	cloned.Collections = append([]domain.CollectionReport(nil), report.Collections...)
	for index := range cloned.Collections {
		cloned.Collections[index].Warnings = append([]string(nil), report.Collections[index].Warnings...)
	}
	return cloned
}
