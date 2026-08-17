package promquery

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientDiscoversAndQueriesMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/label/__name__/values":
			_, _ = writer.Write([]byte(`{"status":"success","data":["milvus_build_info","milvus_proxy_req_count"]}`))
		case "/api/v1/query":
			_, _ = writer.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"role_name":"proxy"},"value":[100,"2"]}]}}`))
		case "/api/v1/query_range":
			_, _ = writer.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"role_name":"proxy"},"values":[[100,"1"],[130,"2"]]}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	names, err := client.MetricNames(context.Background())
	require.NoError(t, err)
	require.Contains(t, names, "milvus_build_info")

	samples, err := client.Instant(context.Background(), "sum(test)", time.Unix(100, 0))
	require.NoError(t, err)
	require.Equal(t, 2.0, samples[0].Value)
	require.Equal(t, "proxy", samples[0].Labels["role_name"])

	series, err := client.Range(context.Background(), "sum(test)", time.Unix(100, 0), time.Unix(130, 0), 30*time.Second)
	require.NoError(t, err)
	require.Len(t, series[0].Points, 2)
	require.Equal(t, int64(130), series[0].Points[1].Timestamp)
}
