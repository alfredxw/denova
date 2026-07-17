package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"denova/config"
)

func TestNewWebSearchToolsRegistersWebSearch(t *testing.T) {
	tools, err := newWebSearchTools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected one web search tool, got %d", len(tools))
	}
	if _, ok := tools[0].(tool.InvokableTool); !ok {
		t.Fatalf("web search tool should be invokable: %T", tools[0])
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != config.AgentToolWebSearch {
		t.Fatalf("expected tool name %q, got %q", config.AgentToolWebSearch, info.Name)
	}
}

func truncateForLog(s string, n int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if n > 0 && len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// 集成测试，用于人工检查四引擎聚合效果。默认测试套件不访问公网；显式设置
// DENOVA_LIVE_WEB_SEARCH_TEST=1 后单独运行本测试。
func TestLiveWebSearch_HelloWorld(t *testing.T) {
	if os.Getenv("DENOVA_LIVE_WEB_SEARCH_TEST") != "1" {
		t.Skip("skipping live web search; set DENOVA_LIVE_WEB_SEARCH_TEST=1 to execute")
	}

	const query = "Hello World"
	agg := newDefaultWebSearchAggregator()
	t.Logf("查询: %q（共 %d 个引擎，逐引擎并发探测）", query, len(agg.engines))

	for _, o := range agg.fanOut(context.Background(), webSearchRequest{Query: query}) {
		if o.err != nil {
			t.Logf("[%s] 失败: %v", o.name, o.err)
			continue
		}
		t.Logf("[%s] 返回 %d 条", o.name, len(o.results))
		for i, r := range o.results {
			t.Logf("  %d. %s\n     %s\n     %s", i+1, r.Title, r.URL, truncateForLog(r.Summary, 120))
		}
	}

	resp := agg.run(context.Background(), webSearchRequest{Query: query})
	t.Logf("聚合 message: %s", resp.Message)
	t.Logf("聚合结果共 %d 条:", len(resp.Results))
	for i, r := range resp.Results {
		t.Logf("  #%d [%s] %s\n       %s\n       %s", i+1, r.Engine, r.Title, r.URL, truncateForLog(r.Summary, 120))
	}
}
