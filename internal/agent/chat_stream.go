package agent

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// processStreamingEvent 处理流式助手消息，输出领域事件。
// 工具调用在流中一检测到名称就立即 emit，让前端尽早展示 running 卡片。
// 参数在流中逐帧 emit tool_args_delta，调用方可在对外传输前按展示策略过滤。
func processStreamingEvent(ctx context.Context, mv *adk.MessageVariant, fullContent, fullThinking *strings.Builder, idleTimeout time.Duration, toolResultMaxBytes int, meta agentEventMetadata, emit func(Event)) (*schema.Message, error) {
	mv.MessageStream.SetAutomaticClose()
	defer mv.MessageStream.Close()
	var accumulatedToolCalls []schema.ToolCall
	emittedTools := make(map[int]bool) // 按 index 记录已 emit tool_call 的工具
	lastArgsLen := make(map[int]int)   // 记录上次已发送的参数长度
	loggedToolPaths := make(map[int]bool)
	var chunks []*schema.Message

	for {
		frame, err := recvMessageFrame(ctx, mv.MessageStream, idleTimeout)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Printf("[agent-run] interrupted reason=stream_recv_error err=%v generated_bytes=%d", err, fullContent.Len())
			if ctx.Err() == nil {
				emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
			}
			return nil, err
		}
		if frame == nil {
			continue
		}
		chunks = append(chunks, frame)
		if frame.ReasoningContent != "" {
			if fullThinking != nil && !meta.SubAgent {
				fullThinking.WriteString(frame.ReasoningContent)
			}
			emit(Event{Type: "thinking", Data: meta.appendTo(map[string]interface{}{"content": frame.ReasoningContent})})
		}
		if frame.Content != "" {
			if !meta.SubAgent {
				fullContent.WriteString(frame.Content)
			}
			emit(Event{Type: "chunk", Data: meta.appendTo(map[string]interface{}{"content": frame.Content})})
		}
		if len(frame.ToolCalls) > 0 {
			accumulatedToolCalls = mergeToolCalls(accumulatedToolCalls, frame.ToolCalls)
			for i, tc := range accumulatedToolCalls {
				if tc.Function.Name == "" {
					continue
				}
				// 首次检测到工具名称，emit tool_call
				if !emittedTools[i] {
					emittedTools[i] = true
					lastArgsLen[i] = 0
					logToolCall(tc.Function.Name, tc.ID, len(tc.Function.Arguments), "streaming")
					manifest := manifestForToolEvent(tc.Function.Name, toolResultMaxBytes)
					data := meta.appendTo(map[string]interface{}{
						"id":                  tc.ID,
						"name":                tc.Function.Name,
						"args":                "",
						"source":              string(manifest.Source),
						"mutates_workspace":   manifest.MutatesWorkspace,
						"requires_post_check": manifest.RequiresPostCheck,
						"max_result_bytes":    manifest.MaxResultBytes,
					})
					if tc.Index != nil {
						data["index"] = *tc.Index
					}
					emit(Event{Type: "tool_call", Data: data})
				}
				// 参数有增量时 emit tool_args_delta
				currentLen := len(tc.Function.Arguments)
				if currentLen > lastArgsLen[i] {
					delta := tc.Function.Arguments[lastArgsLen[i]:currentLen]
					lastArgsLen[i] = currentLen
					if !loggedToolPaths[i] {
						if path := toolPathFromArgs(tc.Function.Arguments); path != "" {
							logToolPath(tc.Function.Name, tc.ID, path)
							loggedToolPaths[i] = true
							emit(Event{Type: "tool_target", Data: meta.appendTo(map[string]interface{}{
								"id":     tc.ID,
								"name":   tc.Function.Name,
								"target": path,
							})})
						}
					}
					data := meta.appendTo(map[string]interface{}{
						"id":    tc.ID,
						"name":  tc.Function.Name,
						"delta": delta,
					})
					if tc.Index != nil {
						data["index"] = *tc.Index
					}
					emit(Event{Type: "tool_args_delta", Data: data})
				}
			}
		}
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		log.Printf("[agent-run] concat streaming message failed err=%v chunks=%d", err, len(chunks))
		return nil, nil
	}
	return msg, nil
}

// processNonStreamingEvent 处理非流式助手消息，输出领域事件。
func processNonStreamingEvent(mv *adk.MessageVariant, fullContent, fullThinking *strings.Builder, toolResultMaxBytes int, meta agentEventMetadata, emit func(Event)) {
	if mv.Message.ReasoningContent != "" {
		if fullThinking != nil && !meta.SubAgent {
			fullThinking.WriteString(mv.Message.ReasoningContent)
		}
		emit(Event{Type: "thinking", Data: meta.appendTo(map[string]interface{}{"content": mv.Message.ReasoningContent})})
	}
	if mv.Message.Content != "" {
		if !meta.SubAgent {
			fullContent.WriteString(mv.Message.Content)
		}
		emit(Event{Type: "chunk", Data: meta.appendTo(map[string]interface{}{"content": mv.Message.Content})})
	}
	for _, tc := range mv.Message.ToolCalls {
		name := tc.Function.Name
		if name == "" {
			continue
		}
		args := tc.Function.Arguments
		logToolCall(name, tc.ID, len(args), "non_streaming")
		target := toolPathFromArgs(args)
		if path := toolPathFromArgs(args); path != "" {
			logToolPath(name, tc.ID, path)
		}
		manifest := manifestForToolEvent(name, toolResultMaxBytes)
		if len(args) > 200 {
			args = args[:200] + "..."
		}
		data := meta.appendTo(map[string]interface{}{
			"id":                  tc.ID,
			"name":                name,
			"args":                args,
			"source":              string(manifest.Source),
			"mutates_workspace":   manifest.MutatesWorkspace,
			"requires_post_check": manifest.RequiresPostCheck,
			"max_result_bytes":    manifest.MaxResultBytes,
		})
		if target != "" {
			data["target"] = target
		}
		if tc.Index != nil {
			data["index"] = *tc.Index
		}
		emit(Event{Type: "tool_call", Data: data})
	}
}

func manifestForToolEvent(name string, toolResultMaxBytes int) ToolManifest {
	manifest := ManifestForTool(name)
	manifest.MaxResultBytes = normalizeToolResultLimitBytes(toolResultMaxBytes)
	return manifest
}

// drainContent 从 MessageVariant 中提取完整内容。
func drainContent(ctx context.Context, mv *adk.MessageVariant, idleTimeout time.Duration) (string, error) {
	if mv.IsStreaming && mv.MessageStream != nil {
		mv.MessageStream.SetAutomaticClose()
		defer mv.MessageStream.Close()
		var sb strings.Builder
		for {
			chunk, err := recvMessageFrame(ctx, mv.MessageStream, idleTimeout)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return sb.String(), err
			}
			if chunk != nil && chunk.Content != "" {
				sb.WriteString(chunk.Content)
			}
		}
		return sb.String(), nil
	}
	if mv.Message != nil {
		return mv.Message.Content, nil
	}
	return "", nil
}
