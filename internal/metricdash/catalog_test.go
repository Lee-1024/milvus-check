package metricdash

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogHasAllCategoriesAndChineseMetadata(t *testing.T) {
	definitions := Definitions()
	require.GreaterOrEqual(t, len(definitions), 20)
	categories := map[string]bool{}
	for _, definition := range definitions {
		require.NotEmpty(t, definition.TitleZH)
		require.NotEmpty(t, definition.DescriptionZH)
		require.NotEmpty(t, definition.Queries)
		categories[definition.Category] = true
	}
	for _, category := range []string{"overview", "request", "querynode", "storage", "load_index", "components"} {
		require.True(t, categories[category], category)
	}
}

func TestSelectVariantReportsUnsupportedMetric(t *testing.T) {
	definition := Definition{Queries: []QueryVariant{{RequiredMetrics: []string{"new_metric"}, PromQL: "new_metric$selector"}}}
	_, missing, ok := SelectVariant(definition, map[string]struct{}{})
	require.False(t, ok)
	require.Equal(t, []string{"new_metric"}, missing)
}

func TestRenderPromQLUsesFixedJobAndRateWindow(t *testing.T) {
	query := RenderPromQL(`sum(rate(test_total$selector[$rate]))`, "milvus-prod", "5m")
	require.Equal(t, `sum(rate(test_total{job="milvus-prod"}[5m]))`, query)
}
