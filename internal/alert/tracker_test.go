package alert

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"milvus-check/internal/domain"

	"github.com/stretchr/testify/require"
)

type fakeNotifier struct {
	notifications []Notification
	err           error
}

func (f *fakeNotifier) Notify(_ context.Context, notification Notification) error {
	f.notifications = append(f.notifications, notification)
	return f.err
}

func TestTrackerAlertsAfterTimeoutAndRepeats(t *testing.T) {
	now := time.Unix(1000, 0)
	notifier := &fakeNotifier{}
	tracker := NewTracker("milvus:19530", 30*time.Minute, time.Hour, notifier, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tracker.now = func() time.Time { return now }
	report := loadingReport("default", "books", 55)

	tracker.Evaluate(context.Background(), report)
	now = now.Add(29 * time.Minute)
	tracker.Evaluate(context.Background(), report)
	require.Empty(t, notifier.notifications)

	now = now.Add(time.Minute)
	tracker.Evaluate(context.Background(), report)
	require.Len(t, notifier.notifications, 1)
	require.False(t, notifier.notifications[0].Repeated)

	now = now.Add(time.Hour)
	tracker.Evaluate(context.Background(), report)
	require.Len(t, notifier.notifications, 2)
	require.True(t, notifier.notifications[1].Repeated)
}

func TestTrackerRetriesFailedNotificationOnNextEvaluation(t *testing.T) {
	now := time.Unix(1000, 0)
	notifier := &fakeNotifier{err: errors.New("failed")}
	tracker := NewTracker("milvus:19530", time.Minute, time.Hour, notifier, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tracker.now = func() time.Time { return now }
	report := loadingReport("default", "books", 10)

	tracker.Evaluate(context.Background(), report)
	now = now.Add(time.Minute)
	tracker.Evaluate(context.Background(), report)
	now = now.Add(time.Second)
	tracker.Evaluate(context.Background(), report)
	require.Len(t, notifier.notifications, 2)
}

func TestTrackerClearsCollectionThatStopsLoading(t *testing.T) {
	now := time.Unix(1000, 0)
	notifier := &fakeNotifier{}
	tracker := NewTracker("milvus:19530", time.Minute, time.Hour, notifier, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tracker.now = func() time.Time { return now }

	tracker.Evaluate(context.Background(), loadingReport("default", "books", 10))
	now = now.Add(30 * time.Second)
	tracker.Evaluate(context.Background(), domain.CheckReport{Collections: []domain.CollectionReport{{Database: "default", Collection: "books", LoadState: domain.LoadStateLoaded}}})
	now = now.Add(time.Minute)
	tracker.Evaluate(context.Background(), loadingReport("default", "books", 20))
	require.Empty(t, notifier.notifications)
}

func loadingReport(database, collection string, progress int64) domain.CheckReport {
	return domain.CheckReport{Collections: []domain.CollectionReport{{Database: database, Collection: collection, Exists: true, LoadState: domain.LoadStateLoading, LoadProgress: progress}}}
}
