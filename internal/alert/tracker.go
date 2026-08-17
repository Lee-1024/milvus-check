package alert

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"milvus-check/internal/domain"
)

type Notifier interface {
	Notify(context.Context, Notification) error
}

type Notification struct {
	Test          bool
	MilvusAddress string
	Database      string
	Collection    string
	Progress      int64
	LoadingSince  time.Time
	CheckedAt     time.Time
	Duration      time.Duration
	Repeated      bool
}

type loadingState struct {
	startedAt  time.Time
	lastSentAt time.Time
}

// Tracker 维护进程内集合加载计时；服务重启后重新开始计时。
type Tracker struct {
	mu             sync.Mutex
	milvusAddress  string
	loadingTimeout time.Duration
	repeatInterval time.Duration
	notifier       Notifier
	logger         *slog.Logger
	states         map[string]loadingState
	now            func() time.Time
}

func NewTracker(milvusAddress string, loadingTimeout, repeatInterval time.Duration, notifier Notifier, logger *slog.Logger) *Tracker {
	return &Tracker{
		milvusAddress: milvusAddress, loadingTimeout: loadingTimeout, repeatInterval: repeatInterval,
		notifier: notifier, logger: logger, states: make(map[string]loadingState), now: time.Now,
	}
}

func (t *Tracker) Evaluate(ctx context.Context, report domain.CheckReport) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	active := make(map[string]struct{})
	for _, collection := range report.Collections {
		key := collection.Database + "\x00" + collection.Collection
		if collection.LoadState != domain.LoadStateLoading {
			if _, exists := t.states[key]; exists {
				delete(t.states, key)
				t.logger.Info("集合已退出加载状态，清除告警计时", "database", collection.Database, "collection", collection.Collection, "load_state", collection.LoadState)
			}
			continue
		}
		active[key] = struct{}{}
		state, exists := t.states[key]
		if !exists {
			t.states[key] = loadingState{startedAt: now}
			t.logger.Info("开始跟踪加载中的集合", "database", collection.Database, "collection", collection.Collection, "progress", collection.LoadProgress)
			continue
		}
		duration := now.Sub(state.startedAt)
		if duration < t.loadingTimeout || (!state.lastSentAt.IsZero() && now.Sub(state.lastSentAt) < t.repeatInterval) {
			continue
		}
		repeated := !state.lastSentAt.IsZero()
		notification := Notification{
			MilvusAddress: t.milvusAddress, Database: collection.Database, Collection: collection.Collection,
			Progress: collection.LoadProgress, LoadingSince: state.startedAt, CheckedAt: now, Duration: duration, Repeated: repeated,
		}
		if err := t.notifier.Notify(ctx, notification); err != nil {
			t.logger.Error("飞书集合加载告警发送失败", "database", collection.Database, "collection", collection.Collection, "duration", duration, "error", err)
			continue
		}
		state.lastSentAt = now
		t.states[key] = state
		t.logger.Warn("飞书集合加载告警发送成功", "database", collection.Database, "collection", collection.Collection, "duration", duration, "repeated", repeated)
	}

	for key := range t.states {
		if _, exists := active[key]; !exists {
			delete(t.states, key)
		}
	}
}
