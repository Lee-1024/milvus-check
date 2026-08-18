package metricdash

import (
	"strconv"
	"strings"
)

type Definition struct {
	ID             string         `json:"id"`
	Category       string         `json:"category"`
	TitleZH        string         `json:"title"`
	DescriptionZH  string         `json:"description"`
	Interpretation string         `json:"interpretation"`
	Source         string         `json:"source"`
	Unit           string         `json:"unit"`
	Visualization  string         `json:"visualization"`
	Queries        []QueryVariant `json:"-"`
}

type QueryVariant struct {
	RequiredMetrics []string
	PromQL          string
}

func Definitions() []Definition {
	return []Definition{
		def("milvus_up", "overview", "Milvus 抓取状态", "Prometheus 是否能抓取 Milvus 指标。", "1 表示可抓取，0 表示抓取失败。", "Prometheus", "bool", "line", "up", `max(up{$match})`),
		def("request_qps", "overview", "总请求速率", "Proxy 每秒接收的请求总数。", "突增通常表示业务流量上升。", "Proxy", "次/秒", "line", "milvus_proxy_req_count", `sum(rate(milvus_proxy_req_count{$match,status="total"}[$rate]))`),
		def("search_qps", "overview", "向量搜索速率", "每秒执行的向量 Search 请求数。", "无数据可能表示当前窗口没有 Search 流量。", "Proxy", "次/秒", "line", "milvus_proxy_req_count", `sum(rate(milvus_proxy_req_count{$match,status="total",function_name="Search"}[$rate]))`),
		def("query_qps", "overview", "标量查询速率", "每秒执行的 Query 请求数。", "用于观察标量查询负载。", "Proxy", "次/秒", "line", "milvus_proxy_req_count", `sum(rate(milvus_proxy_req_count{$match,status="total",function_name="Query"}[$rate]))`),
		def("request_success_rate", "overview", "请求成功率", "成功请求占全部请求的百分比。", "持续低于 99% 时应检查错误日志。", "Proxy", "%", "line", "milvus_proxy_req_count", `clamp_max(100 * sum(rate(milvus_proxy_req_count{$match,status="success"}[$rate])) / clamp_min(sum(rate(milvus_proxy_req_count{$match,status="total"}[$rate])), 1e-9), 100)`),
		hist("request_p95", "overview", "请求 P95 延迟", "95% 的 Proxy 请求在该时间内完成。", "Proxy", "milvus_proxy_req_latency_bucket", 0.95),
		hist("request_p99", "request", "请求 P99 延迟", "99% 的 Proxy 请求在该时间内完成。", "Proxy", "milvus_proxy_req_latency_bucket", 0.99),
		def("proxy_tt_lag", "request", "Proxy 时间同步延迟", "Proxy 时间戳与当前时间的差值。", "持续升高可能表示消息消费或时间同步阻塞。", "Proxy", "ms", "line", "milvus_proxy_tt_lag_ms", `max(milvus_proxy_tt_lag_ms{$match})`),
		def("rate_limited_qps", "request", "限流请求速率", "每秒被 Proxy 限流的请求数。", "非零表示请求触发了限流策略。", "Proxy", "次/秒", "line", "milvus_proxy_rate_limit_req_count", `sum(rate(milvus_proxy_rate_limit_req_count{$match}[$rate]))`),
		def("proxy_queue_tasks", "request", "Proxy 队列任务数", "Proxy 各状态请求队列中的任务数量。", "持续增长表示处理速度低于进入速度。", "Proxy", "个", "bars", "milvus_proxy_queue_task_num", `sum by (queue_type,task_state) (milvus_proxy_queue_task_num{$match})`),
		hist("querynode_request_p95", "querynode", "查询节点请求 P95", "QueryNode Search/Query 请求的 P95 延迟。", "QueryNode", "milvus_querynode_sq_req_latency_bucket", 0.95),
		hist("querynode_queue_p95", "querynode", "查询节点排队 P95", "Search/Query 任务在 QueryNode 队列中的等待时间。", "QueryNode", "milvus_querynode_sq_queue_latency_bucket", 0.95),
		hist("querynode_core_p95", "querynode", "查询节点处理 P95", "QueryNode 核心查询阶段的 P95 耗时。", "QueryNode", "milvus_querynode_sq_core_latency_bucket", 0.95),
		def("querynode_ready_tasks", "querynode", "查询节点就绪任务", "QueryNode 已进入可执行状态的读取任务数。", "持续较高表示执行资源可能不足。", "QueryNode", "个", "line", "milvus_querynode_read_task_ready_len", `sum(milvus_querynode_read_task_ready_len{$match})`),
		def("querynode_unsolved_tasks", "querynode", "查询节点未解决任务", "QueryNode 尚未满足执行条件的任务数。", "持续积压需结合时间同步和 Segment 状态排查。", "QueryNode", "个", "line", "milvus_querynode_read_task_unsolved_len", `sum(milvus_querynode_read_task_unsolved_len{$match})`),
		def("insert_vectors_rate", "storage", "写入向量速率", "Proxy 每秒接收的写入向量数量。", "反映实际写入数据量，不等同于 Insert 请求数。", "Proxy", "向量/秒", "line", "milvus_proxy_insert_vectors_count", `sum(rate(milvus_proxy_insert_vectors_count{$match}[$rate]))`),
		def("segment_count", "storage", "Segment 数量", "DataCoord 按状态和层级统计的 Segment 数。", "Segment 数持续过高可能增加调度和查询开销。", "DataCoord", "个", "bars", "milvus_datacoord_segment_num", `sum by (segment_state,segment_level) (milvus_datacoord_segment_num{$match})`),
		def("stored_rows", "storage", "已存储行数", "DataCoord 记录的各集合 Segment 行数。", "用于观察数据库和集合的数据规模。", "DataCoord", "行", "bars", "milvus_datacoord_stored_rows_num", `sum by (db_name,collection_name,segment_state) (milvus_datacoord_stored_rows_num{$match})`),
		def("stored_binlog_bytes", "storage", "Binlog 存储量", "DataCoord 记录的 Binlog 文件总字节数。", "用于观察对象存储数据增长。", "DataCoord", "bytes", "line", "milvus_datacoord_stored_binlog_size", `sum(milvus_datacoord_stored_binlog_size{$match})`),
		def("load_success_rate", "load_index", "集合加载成功率", "QueryCoord 集合加载请求的成功比例。", "失败时结合集合加载表和 QueryCoord 日志排查。", "QueryCoord", "%", "line", "milvus_querycoord_load_req_count", `clamp_max(100 * sum(rate(milvus_querycoord_load_req_count{$match,status="success"}[$rate])) / clamp_min(sum(rate(milvus_querycoord_load_req_count{$match,status="total"}[$rate])), 1e-9), 100)`),
		hist("collection_load_p95", "load_index", "集合加载 P95 耗时", "QueryCoord 完整集合加载请求的 P95 耗时。", "QueryCoord", "milvus_querycoord_load_latency_bucket", 0.95),
		hist("segment_load_p95", "load_index", "Segment 加载 P95 耗时", "QueryNode 加载 Segment 的 P95 耗时。", "QueryNode", "milvus_querynode_load_segment_latency_bucket", 0.95),
		hist("index_load_p95", "load_index", "索引加载 P95 耗时", "QueryNode 加载索引的 P95 耗时。", "QueryNode", "milvus_querynode_load_index_latency_bucket", 0.95),
		def("component_nodes", "components", "组件节点数", "Milvus 各角色当前暴露指标的节点数量。", "节点数与部署拓扑不一致时应检查组件状态。", "Milvus", "个", "bars", "milvus_num_node", `count by (role_name) (milvus_num_node{$match})`),
		def("goroutines", "components", "Go 协程数", "Milvus 各进程当前 Go 协程总数。", "只用于趋势观察，合理范围取决于负载。", "Go runtime", "个", "line", "go_goroutines", `sum(go_goroutines{$match})`),
		def("resident_memory", "components", "进程常驻内存", "Milvus 各进程常驻内存总量。", "需结合容器内存限制和 QueryNode 高水位判断。", "Process", "bytes", "line", "process_resident_memory_bytes", `sum(process_resident_memory_bytes{$match})`),
	}
}

func def(id, category, title, description, interpretation, source, unit, visualization, metric, promql string) Definition {
	return Definition{ID: id, Category: category, TitleZH: title, DescriptionZH: description, Interpretation: interpretation, Source: source, Unit: unit, Visualization: visualization, Queries: []QueryVariant{{RequiredMetrics: []string{metric}, PromQL: promql}}}
}

func hist(id, category, title, description, source, metric string, quantile float64) Definition {
	query := `histogram_quantile(` + strconv.FormatFloat(quantile, 'f', 2, 64) + `, sum by (le) (rate(` + metric + `{$match}[$rate])))`
	return def(id, category, title, description, "分位数越高表示慢请求尾部越明显。", source, "ms", "line", metric, query)
}

func SelectVariant(definition Definition, names map[string]struct{}) (QueryVariant, []string, bool) {
	var firstMissing []string
	for _, variant := range definition.Queries {
		missing := make([]string, 0)
		for _, required := range variant.RequiredMetrics {
			if _, ok := names[required]; !ok {
				missing = append(missing, required)
			}
		}
		if len(missing) == 0 {
			return variant, nil, true
		}
		if firstMissing == nil {
			firstMissing = missing
		}
	}
	return QueryVariant{}, firstMissing, false
}

func RenderPromQL(template, job, rateWindow string) string {
	match := `job=` + strconv.Quote(job)
	result := strings.ReplaceAll(template, "$selector", "{"+match+"}")
	result = strings.ReplaceAll(result, "$match", match)
	return strings.ReplaceAll(result, "$rate", rateWindow)
}
