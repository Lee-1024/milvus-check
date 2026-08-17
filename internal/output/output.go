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
		if err := encoder.Encode(report); err != nil {
			return fmt.Errorf("输出 JSON: %w", err)
		}
		return nil
	}
	return writeTable(writer, report)
}

func writeTable(writer io.Writer, report domain.CheckReport) error {
	tab := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tab, "DATABASE\tCOLLECTION\tEXISTS\tLOAD_STATE\tPROGRESS\tENTITIES\tPARTITIONS\tINDEX_OK\tERROR"); err != nil {
		return err
	}
	for _, item := range report.Collections {
		if _, err := fmt.Fprintf(tab, "%s\t%s\t%t\t%s\t%d%%\t%d\t%d\t%t\t%s\n",
			item.Database, item.Collection, item.Exists, item.LoadState, item.LoadProgress,
			item.EntityCount, item.PartitionCount, item.IndexHealthy, item.Error); err != nil {
			return err
		}
	}
	return tab.Flush()
}
