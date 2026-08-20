package alert

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFeishuNotifierSendsInteractiveCard(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Contains(t, request.Header.Get("Content-Type"), "application/json")
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer server.Close()

	notifier := NewFeishuNotifier(server.URL, time.Second)
	err := notifier.Notify(context.Background(), Notification{MilvusAddress: "milvus:19530", Database: "default", Collection: "books", Progress: 42, LoadingSince: time.Unix(100, 0), CheckedAt: time.Unix(3700, 0), Duration: time.Hour})

	require.NoError(t, err)
	require.Equal(t, "interactive", payload["msg_type"])
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "default")
	require.Contains(t, string(encoded), "books")
	require.Contains(t, string(encoded), "42%")
}

func TestFeishuNotifierSendsMultipleNotificationsInOneCard(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	err := NewFeishuNotifier(server.URL, time.Second).NotifyBatch(context.Background(), []Notification{
		{Database: "db1", Collection: "a", Progress: 10},
		{Database: "db2", Collection: "b", Progress: 20},
	})
	require.NoError(t, err)
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "db1")
	require.Contains(t, string(encoded), "db2")
}

func TestFeishuNotifierRejectsHTTPAndBusinessErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http error", http.StatusBadGateway, `bad gateway`},
		{"business error", http.StatusOK, `{"code":19001,"msg":"invalid webhook"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			err := NewFeishuNotifier(server.URL, time.Second).Notify(context.Background(), Notification{})
			require.Error(t, err)
			require.NotContains(t, err.Error(), server.URL)
		})
	}
}

func TestFeishuNotifierTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = writer.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	err := NewFeishuNotifier(server.URL, 10*time.Millisecond).Notify(context.Background(), Notification{})
	require.Error(t, err)
}
