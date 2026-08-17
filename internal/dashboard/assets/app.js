(() => {
  "use strict";

  const elements = {
    checkedAt: document.getElementById("checked-at"),
    countdown: document.getElementById("countdown"),
    refreshButton: document.getElementById("refresh-button"),
    overallDot: document.getElementById("overall-dot"),
    overallState: document.getElementById("overall-state"),
    statusDetail: document.getElementById("status-detail"),
    errorBanner: document.getElementById("error-banner"),
    staleBanner: document.getElementById("stale-banner"),
    body: document.getElementById("collection-body"),
    empty: document.getElementById("empty-state"),
    resultCount: document.getElementById("result-count"),
    pagination: document.getElementById("collection-pagination"),
    pageInfo: document.getElementById("collection-page-info"),
    pageSize: document.getElementById("collection-page-size"),
    pageButtons: document.getElementById("collection-page-buttons"),
    firstPage: document.getElementById("collection-first-page"),
    prevPage: document.getElementById("collection-prev-page"),
    nextPage: document.getElementById("collection-next-page"),
    lastPage: document.getElementById("collection-last-page")
  };

  let intervalSeconds = 30;
  let remainingSeconds = intervalSeconds;
  let lastStatus = null;
  let refreshing = false;
  let allCollections = [];
  let currentPage = 1;
  let pageSize = 20;

  const metricIDs = ["database-count", "collection-count", "loaded-count", "loading-count", "not-loaded-count", "error-count", "search-qps", "query-qps", "failed-ps"];

  function setText(id, value) {
    document.getElementById(id).textContent = value;
  }

  function formatNumber(value, digits = 0) {
    if (value === null || value === undefined || Number.isNaN(Number(value))) return "--";
    return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: digits }).format(Number(value));
  }

  function stateText(state) {
    return ({ loaded: "已加载", loading: "加载中", not_load: "未加载", unknown: "未知" })[state] || "未知";
  }

  function createCell(label, content, className = "") {
    const cell = document.createElement("td");
    cell.dataset.label = label;
    if (className) cell.className = className;
    if (content instanceof Node) cell.appendChild(content);
    else cell.textContent = content;
    return cell;
  }

  function renderRows(collections) {
    elements.body.replaceChildren();

    collections.forEach((item) => {
      const row = document.createElement("tr");

      const identity = document.createElement("div");
      const collection = document.createElement("span");
      collection.className = "collection-name";
      collection.textContent = item.collection;
      const database = document.createElement("span");
      database.className = "database-name";
      database.textContent = item.database;
      identity.append(collection, database);
      row.appendChild(createCell("数据库 / 集合", identity));

      const state = document.createElement("span");
      state.className = `status-label status-${item.load_state || "unknown"}`;
      state.textContent = stateText(item.load_state);
      row.appendChild(createCell("加载状态", state));

      const progress = document.createElement("div");
      progress.className = "progress";
      const track = document.createElement("div");
      track.className = "progress-track";
      const fill = document.createElement("div");
      fill.className = "progress-fill";
      fill.style.width = `${Math.max(0, Math.min(100, item.load_progress_percent || 0))}%`;
      track.appendChild(fill);
      const value = document.createElement("span");
      value.className = "progress-value";
      value.textContent = `${item.load_progress_percent || 0}%`;
      progress.append(track, value);
      row.appendChild(createCell("进度", progress));

      row.appendChild(createCell("实体数", formatNumber(item.entity_count), "number"));
      row.appendChild(createCell("分区", formatNumber(item.partition_count), "number"));
      row.appendChild(createCell("索引", item.index_healthy ? "正常" : "异常", item.index_healthy ? "index-ok" : "index-bad"));
      row.appendChild(createCell("已加载实体", formatNumber(item.metrics?.loaded_entities), "number"));
      row.appendChild(createCell("Segment", formatNumber(item.metrics?.segment_count), "number"));

      const notes = [];
      if (item.error) notes.push(item.error);
      if (Array.isArray(item.warnings)) notes.push(...item.warnings);
      row.appendChild(createCell("说明", notes.join("；") || "--", item.error ? "note note-error" : "note"));
      elements.body.appendChild(row);
    });
  }

  function renderCollectionPage() {
    const total = allCollections.length;
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    currentPage = Math.max(1, Math.min(currentPage, totalPages));
    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = Math.min(startIndex + pageSize, total);

    elements.resultCount.textContent = `${total} 条`;
    elements.empty.hidden = total !== 0;
    elements.pagination.hidden = total === 0;
    renderRows(allCollections.slice(startIndex, endIndex));
    if (total === 0) return;

    elements.pageInfo.textContent = `第 ${startIndex + 1}-${endIndex} 条，共 ${total} 条`;
    elements.firstPage.disabled = currentPage === 1;
    elements.prevPage.disabled = currentPage === 1;
    elements.nextPage.disabled = currentPage === totalPages;
    elements.lastPage.disabled = currentPage === totalPages;
    elements.pageButtons.replaceChildren();

    let firstVisible = Math.max(1, currentPage - 2);
    let lastVisible = Math.min(totalPages, firstVisible + 4);
    firstVisible = Math.max(1, lastVisible - 4);
    for (let page = firstVisible; page <= lastVisible; page += 1) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = String(page);
      button.classList.toggle("active", page === currentPage);
      button.setAttribute("aria-label", `第 ${page} 页`);
      button.setAttribute("aria-current", page === currentPage ? "page" : "false");
      button.addEventListener("click", () => { currentPage = page; renderCollectionPage(); });
      elements.pageButtons.appendChild(button);
    }
  }

  function renderSummary(status) {
    const collections = status.collections || [];
    const databases = new Set(collections.map((item) => item.database));
    const loaded = collections.filter((item) => item.load_state === "loaded").length;
    const loading = collections.filter((item) => item.load_state === "loading").length;
    const notLoaded = collections.filter((item) => item.load_state === "not_load").length;
    const errors = collections.filter((item) => Boolean(item.error)).length;
    const metrics = collections.find((item) => item.metrics)?.metrics || {};

    setText("database-count", formatNumber(databases.size));
    setText("collection-count", formatNumber(collections.length));
    setText("loaded-count", formatNumber(loaded));
    setText("loading-count", formatNumber(loading));
    setText("not-loaded-count", formatNumber(notLoaded));
    setText("error-count", formatNumber(errors));
    setText("search-qps", formatNumber(metrics.search_qps, 2));
    setText("query-qps", formatNumber(metrics.query_qps, 2));
    setText("failed-ps", formatNumber(metrics.failed_request_ps, 3));
  }

  function renderStatus(status) {
    lastStatus = status;
    intervalSeconds = Math.max(1, status.refresh_interval_seconds || 30);
    remainingSeconds = intervalSeconds;
    elements.overallDot.className = "state-dot";

    if (!status.ready || !status.up) {
      elements.overallDot.classList.add("state-bad");
      elements.overallState.textContent = status.ready ? "采集不可用" : "等待首次检查";
      elements.statusDetail.textContent = status.last_error || "后台尚未生成可用快照。";
    } else if (status.healthy) {
      elements.overallDot.classList.add("state-good");
      elements.overallState.textContent = "运行正常";
      elements.statusDetail.textContent = "所有集合满足当前加载阈值。";
    } else {
      elements.overallDot.classList.add("state-warn");
      elements.overallState.textContent = "需要关注";
      elements.statusDetail.textContent = "存在未加载、加载中或检查异常的集合。";
    }

    const checkedAt = new Date(status.checked_at);
    elements.checkedAt.textContent = Number.isNaN(checkedAt.getTime()) ? "尚未检查" : `检查于 ${checkedAt.toLocaleString("zh-CN")}`;
    elements.errorBanner.hidden = !status.last_error;
    elements.errorBanner.textContent = status.last_error || "";
    elements.staleBanner.hidden = Number.isNaN(checkedAt.getTime()) || Date.now() - checkedAt.getTime() <= intervalSeconds * 2000;
    renderSummary(status);
    allCollections = status.collections || [];
    renderCollectionPage();
  }

  async function refresh() {
    if (refreshing) return;
    refreshing = true;
    elements.refreshButton.classList.add("is-loading");
    try {
      const response = await fetch("/api/status", { cache: "no-store" });
      const status = await response.json();
      renderStatus(status);
    } catch (error) {
      elements.errorBanner.hidden = false;
      elements.errorBanner.textContent = `状态接口请求失败：${error.message}`;
      if (!lastStatus) {
        metricIDs.forEach((id) => setText(id, "--"));
        allCollections = [];
        renderCollectionPage();
      }
    } finally {
      refreshing = false;
      elements.refreshButton.classList.remove("is-loading");
    }
  }

  elements.refreshButton.addEventListener("click", refresh);
  elements.pageSize.addEventListener("change", () => {
    pageSize = Number(elements.pageSize.value) || 20;
    currentPage = 1;
    renderCollectionPage();
  });
  elements.firstPage.addEventListener("click", () => { currentPage = 1; renderCollectionPage(); });
  elements.prevPage.addEventListener("click", () => { currentPage -= 1; renderCollectionPage(); });
  elements.nextPage.addEventListener("click", () => { currentPage += 1; renderCollectionPage(); });
  elements.lastPage.addEventListener("click", () => {
    currentPage = Math.max(1, Math.ceil(allCollections.length / pageSize));
    renderCollectionPage();
  });
  setInterval(() => {
    remainingSeconds -= 1;
    if (remainingSeconds <= 0) {
      remainingSeconds = intervalSeconds;
      refresh();
    }
    elements.countdown.textContent = `${remainingSeconds}s 后刷新`;
  }, 1000);

  const categoryNames = {
    overview: "总览", request: "请求与延迟", querynode: "查询节点",
    storage: "写入与存储", load_index: "加载与索引", components: "组件状态"
  };
  const stateNames = { available: "有数据", zero: "当前为零", no_data: "时间范围内无数据", unsupported: "当前版本不支持", error: "查询失败", disabled: "未启用" };
  const palette = ["#4fc3a1", "#65c7d0", "#e5ad45", "#f16f65", "#9ea7ff", "#d28bdc"];
  const metricUI = {
    nav: document.getElementById("metric-nav"), range: document.getElementById("range-control"), grid: document.getElementById("metric-grid"),
    dictionary: document.getElementById("metric-dictionary"), dictionaryBody: document.getElementById("dictionary-body"), search: document.getElementById("dictionary-search"),
    message: document.getElementById("metrics-message"), build: document.getElementById("build-info"), dialog: document.getElementById("metric-dialog")
  };
  let metricCatalog = [];
  let activeCategory = "overview";
  let activeRange = "1h";
  let metricRequest = null;
  const charts = new Map();

  function setMetricValue(element, value, unit) {
    element.replaceChildren();
    if (value === null || value === undefined) {
      element.textContent = "--";
      return;
    }
    const formatted = formatMetricValue(value, unit);
    element.append(document.createTextNode(`${formatted.value} `));
    const suffix = document.createElement("small"); suffix.textContent = formatted.unit;
    element.appendChild(suffix);
  }

  function formatMetricValue(value, unit) {
    if (unit !== "bytes") {
      const digits = Math.abs(value) < 10 ? 2 : 1;
      return { value: formatNumber(value, digits), unit: unit || "" };
    }
    const units = ["B", "KiB", "MiB", "GiB", "TiB"];
    let scaled = Math.abs(value);
    let index = 0;
    while (scaled >= 1024 && index < units.length - 1) {
      scaled /= 1024;
      index += 1;
    }
    if (value < 0) scaled = -scaled;
    return { value: formatNumber(scaled, scaled < 10 ? 2 : 1), unit: units[index] };
  }

  function formatAxisValue(value, unit) {
    if (unit === "bytes") {
      const formatted = formatMetricValue(value, unit);
      return `${formatted.value} ${formatted.unit}`;
    }
    if (Math.abs(value) >= 10000) {
      return new Intl.NumberFormat("zh-CN", { notation: "compact", maximumFractionDigits: 1 }).format(value);
    }
    return formatNumber(value, Math.abs(value) < 10 ? 1 : 0);
  }

  function seriesName(labels, index) {
    const parts = Object.entries(labels || {}).filter(([key]) => !["job", "instance", "node_id"].includes(key)).map(([key, value]) => `${key}=${value}`);
    return parts.join(" · ") || (index === 0 ? "全部" : `序列 ${index + 1}`);
  }

  function chartData(series) {
    const timestamps = [...new Set(series.flatMap((item) => (item.points || []).map((point) => point.timestamp)))].sort((a, b) => a - b);
    const data = [timestamps];
    series.forEach((item) => {
      const values = new Map((item.points || []).map((point) => [point.timestamp, point.value]));
      data.push(timestamps.map((timestamp) => values.has(timestamp) ? values.get(timestamp) : null));
    });
    return data;
  }

  function drawChart(host, metric) {
    const old = charts.get(metric.id);
    if (old) old.destroy();
    host.replaceChildren();
    if (!window.uPlot || !metric.series?.some((item) => item.points?.length)) {
      host.textContent = metric.message || stateNames[metric.state] || "暂无趋势数据";
      return;
    }
    const series = metric.series.slice(0, 6);
    const width = Math.max(280, host.clientWidth || 520);
    const plotHost = document.createElement("div");
    plotHost.className = "plot-host";
    const legend = document.createElement("div");
    legend.className = "chart-legend";
    series.forEach((item, index) => {
      const name = seriesName(item.labels, index);
      const entry = document.createElement("span"); entry.className = "chart-legend-item"; entry.title = name;
      const swatch = document.createElement("i"); swatch.style.backgroundColor = palette[index];
      const label = document.createElement("span"); label.textContent = name;
      entry.append(swatch, label); legend.appendChild(entry);
    });
    host.append(plotHost, legend);
    const options = {
      width, height: 235,
      legend: { show: false },
      cursor: { drag: { x: true, y: false } },
      scales: { x: { time: true } },
      axes: [
        { stroke: "#a9b2ad", grid: { stroke: "#303532" } },
        { size: 82, stroke: "#a9b2ad", grid: { stroke: "#303532" }, values: (_plot, values) => values.map((value) => formatAxisValue(value, metric.unit)) }
      ],
      series: [{ label: "时间" }, ...series.map((item, index) => ({
        label: seriesName(item.labels, index), stroke: palette[index], width: 2, points: { show: false },
        value: (_plot, value) => value === null ? "--" : `${formatMetricValue(value, metric.unit).value} ${formatMetricValue(value, metric.unit).unit}`
      }))]
    };
    charts.set(metric.id, new window.uPlot(options, chartData(series), plotHost));
  }

  function showMetricDetail(metric) {
    document.getElementById("dialog-category").textContent = categoryNames[metric.category] || metric.category;
    document.getElementById("dialog-title").textContent = metric.title;
    document.getElementById("dialog-description").textContent = metric.description;
    document.getElementById("dialog-interpretation").textContent = metric.interpretation;
    document.getElementById("dialog-source").textContent = metric.source;
    document.getElementById("dialog-state").textContent = stateNames[metric.state] || metric.state;
    document.getElementById("dialog-promql").textContent = metric.promql || "当前版本没有可用查询";
    metricUI.dialog.showModal();
  }

  function createMetricPanel(metric) {
    const panel = document.createElement("article");
    panel.className = "metric-panel";
    panel.dataset.metricId = metric.id;
    const header = document.createElement("div"); header.className = "metric-panel-header";
    const titleBlock = document.createElement("div");
    const title = document.createElement("h3"); title.textContent = metric.title;
    const value = document.createElement("strong"); value.className = "metric-value"; setMetricValue(value, metric.current, metric.unit);
    const state = document.createElement("span"); state.className = `metric-state level-${metric.level || "info"}`; state.textContent = stateNames[metric.state] || metric.state;
    titleBlock.append(title, value, state);
    const info = document.createElement("button"); info.type = "button"; info.className = "info-button"; info.textContent = "ⓘ"; info.title = "查看指标说明"; info.setAttribute("aria-label", `查看${metric.title}说明`);
    info.addEventListener("click", () => showMetricDetail(metric));
    header.append(titleBlock, info);
    const host = document.createElement("div"); host.className = "chart-host"; host.textContent = "正在加载趋势";
    panel.append(header, host);
    return { panel, host };
  }

  async function loadCategory() {
    metricRequest?.abort();
    metricRequest = new AbortController();
    charts.forEach((chart) => chart.destroy()); charts.clear();
    metricUI.dictionary.hidden = activeCategory !== "dictionary";
    metricUI.grid.hidden = activeCategory === "dictionary";
    if (activeCategory === "dictionary") { renderDictionary(); return; }
    metricUI.grid.replaceChildren();
    const definitions = metricCatalog.filter((item) => item.category === activeCategory);
    for (const definition of definitions) {
      const { panel, host } = createMetricPanel(definition);
      metricUI.grid.appendChild(panel);
      fetch(`/api/metrics/series/${encodeURIComponent(definition.id)}?range=${activeRange}`, { signal: metricRequest.signal, cache: "no-store" })
        .then((response) => response.json()).then((metric) => {
          if (metric.error) throw new Error(metric.error);
          const replacement = createMetricPanel(metric);
          panel.replaceWith(replacement.panel);
          drawChart(replacement.host, metric);
        }).catch((error) => { if (error.name !== "AbortError") host.textContent = `趋势加载失败：${error.message}`; });
    }
  }

  function renderDictionary() {
    const keyword = metricUI.search.value.trim().toLowerCase();
    metricUI.dictionaryBody.replaceChildren();
    metricCatalog.filter((item) => [item.title, item.source, item.description, ...(item.missing_metrics || [])].join(" ").toLowerCase().includes(keyword)).forEach((item) => {
      const row = document.createElement("tr");
      [item.title, categoryNames[item.category], item.source, item.unit, stateNames[item.state]].forEach((value) => row.appendChild(createCell("", value || "--")));
      row.addEventListener("click", () => showMetricDetail(item));
      metricUI.dictionaryBody.appendChild(row);
    });
  }

  async function loadMetricCatalog() {
    try {
      const response = await fetch("/api/metrics/catalog", { cache: "no-store" });
      const catalog = await response.json();
      if (!response.ok) throw new Error(catalog.error || "目录请求失败");
      metricCatalog = catalog.items || [];
      activeRange = catalog.default_range || activeRange;
      metricUI.range.querySelectorAll("button").forEach((button) => button.classList.toggle("active", button.dataset.range === activeRange));
      metricUI.build.textContent = catalog.version ? `Milvus ${catalog.version}${catalog.git_commit ? ` · ${catalog.git_commit.slice(0, 8)}` : ""}` : (catalog.enabled ? "未发现版本信息" : "Prometheus 未启用");
      metricUI.message.hidden = catalog.enabled;
      metricUI.message.textContent = catalog.enabled ? "" : "Prometheus 查询未启用，集合加载检查仍可正常使用。";
      await loadCategory();
    } catch (error) {
      metricUI.message.hidden = false;
      metricUI.message.textContent = `指标目录加载失败：${error.message}`;
    }
  }

  metricUI.nav.addEventListener("click", (event) => {
    const button = event.target.closest("button[data-category]"); if (!button) return;
    activeCategory = button.dataset.category;
    metricUI.nav.querySelectorAll("button").forEach((item) => item.classList.toggle("active", item === button));
    loadCategory();
  });
  metricUI.range.addEventListener("click", (event) => {
    const button = event.target.closest("button[data-range]"); if (!button || button.dataset.range === activeRange) return;
    activeRange = button.dataset.range;
    metricUI.range.querySelectorAll("button").forEach((item) => item.classList.toggle("active", item === button));
    loadCategory();
  });
  metricUI.search.addEventListener("input", renderDictionary);

  refresh();
  loadMetricCatalog();
})();
