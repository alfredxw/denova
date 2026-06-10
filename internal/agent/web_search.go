package agent

import (
	"context"
	"fmt"
	"time"

	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/tool"

	"nova/config"
)

const webSearchToolDescription = "Search the public web for current or external information. Return result titles, URLs, and short summaries; cite useful URLs in the final answer."

func newWebSearchTools(ctx context.Context) ([]tool.BaseTool, error) {
	searchTool, err := duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		ToolName:   config.AgentToolWebSearch,
		ToolDesc:   webSearchToolDescription,
		Region:     duckduckgo.RegionWT,
		MaxResults: 8,
		Timeout:    20 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("创建网页搜索工具失败: %w", err)
	}
	return []tool.BaseTool{searchTool}, nil
}
