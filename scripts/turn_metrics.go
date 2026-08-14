// turn_metrics: 对单个或一对 story jsonl 逐 turn 提取关键指标,
// 可选接受 runs jsonl 统计 llm_call 与 reasoning_tokens。
//
// 用法:
//
//	go run scripts/turn_metrics.go <story.jsonl>
//	go run scripts/turn_metrics.go --compare --before <before.jsonl> --after <after.jsonl>
//	go run scripts/turn_metrics.go <story.jsonl> --runs <run1.jsonl> <run2.jsonl>
//
// 依赖:仅 Go 标准库。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type storyRow struct {
	Turn           int    `json:"turn"`
	TurnID         string `json:"turn_id"`
	NarrativeBytes int    `json:"narrative_bytes"`
	ThinkingBytes  int    `json:"thinking_bytes"`
	ThinkingEvents int    `json:"thinking_events"`
	ToolCalls      int    `json:"tool_calls"`
	Ratio          any    `json:"ratio"`
}

type llmSlot struct {
	LLMCalls        int `json:"llm_calls"`
	ReasoningTokens int `json:"reasoning_tokens"`
}

func parseStory(path string) ([]storyRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024) // 单行可达 ~500 KiB
	var rows []storyRow
	idx := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		idx++
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			fmt.Fprintf(os.Stderr, "行 %d 解析失败: %v\n", idx, err)
			continue
		}
		turnAny, ok := raw["turn"]
		if !ok {
			turnAny, _ = json.Marshal(raw)
		}
		var turn map[string]json.RawMessage
		_ = json.Unmarshal(turnAny, &turn)
		narrative := unmarshalString(turn["narrative"])
		thinking := unmarshalString(turn["thinking"])
		displayEvents := unmarshalArray(turn["display_events"])
		thinkingEvents, toolCalls := 0, 0
		for _, ev := range displayEvents {
			var m map[string]json.RawMessage
			if err := json.Unmarshal(ev, &m); err != nil {
				continue
			}
			role := unmarshalString(m["role"])
			if role == "" {
				role = unmarshalString(m["type"])
			}
			switch role {
			case "thinking":
				thinkingEvents++
			case "tool_call":
				toolCalls++
			}
		}
		turnID := unmarshalString(turn["id"])
		if turnID == "" {
			turnID = unmarshalString(turn["turn_id"])
		}
		nBytes := len(narrative)
		tBytes := len(thinking)
		var ratio any
		if nBytes > 0 {
			ratio = round2(float64(tBytes) / float64(nBytes))
		} else {
			ratio = nil
		}
		rows = append(rows, storyRow{
			Turn:           idx,
			TurnID:         turnID,
			NarrativeBytes: nBytes,
			ThinkingBytes:  tBytes,
			ThinkingEvents: thinkingEvents,
			ToolCalls:      toolCalls,
			Ratio:          ratio,
		})
	}
	return rows, scanner.Err()
}

func unmarshalString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

func unmarshalArray(raw json.RawMessage) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	return nil
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func parseRuns(paths []string) map[string]*llmSlot {
	agg := map[string]*llmSlot{}
	// Normal llm_call trace spans in this project carry no turn_id (only
	// run_id at the record root plus data.attrs). Build a run_id -> turn_id
	// map from run_created / run_context records emitted earlier, then
	// attribute each llm_call to its turn via run_id.
	runToTurn := map[string]string{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "跳过缺失 runs: %s (%v)\n", p, err)
			continue
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var ev struct {
				Type  string `json:"type"`
				RunID string `json:"run_id"`
				Data  struct {
					TurnID string `json:"turn_id"`
					Attrs  struct {
						ReasoningTokens int `json:"reasoning_tokens"`
					} `json:"attrs"`
				} `json:"data"`
			}
			if err := json.Unmarshal(line, &ev); err != nil {
				continue
			}
			switch ev.Type {
			case "run_created", "run_context":
				if ev.Data.TurnID != "" {
					runToTurn[ev.RunID] = ev.Data.TurnID
				}
			case "llm_call":
				turnID := runToTurn[ev.RunID]
				if turnID == "" {
					turnID = "other"
				}
				slot, ok := agg[turnID]
				if !ok {
					slot = &llmSlot{}
					agg[turnID] = slot
				}
				slot.LLMCalls++
				slot.ReasoningTokens += ev.Data.Attrs.ReasoningTokens
			}
		}
		f.Close()
	}
	return agg
}

func formatRow(r storyRow, slot *llmSlot) string {
	ratioStr := "   n/a"
	if r.Ratio != nil {
		ratioStr = fmt.Sprintf("%5.2fx", r.Ratio)
	}
	llmStr := ""
	reasonStr := ""
	if slot != nil {
		llmStr = fmt.Sprintf("%d", slot.LLMCalls)
		reasonStr = fmt.Sprintf("%d", slot.ReasoningTokens)
	}
	return fmt.Sprintf("%4d  | %9d | %8d | %6d | %7s | %10d | %9s | %10s",
		r.Turn, r.NarrativeBytes, r.ThinkingBytes, r.ThinkingEvents,
		ratioStr, r.ToolCalls, llmStr, reasonStr)
}

func printRows(label string, rows []storyRow, llm map[string]*llmSlot) {
	fmt.Printf("\n## %s\n", label)
	fmt.Println("turn  | narrative | thinking | events |  ratio | tool_calls | llm_calls | reason_tok")
	fmt.Println("-----------------------------------------------------------------------------------------")
	for _, r := range rows {
		fmt.Println(formatRow(r, llm[r.TurnID]))
	}
	ratios := []float64{}
	for _, r := range rows {
		if r.Ratio != nil {
			ratios = append(ratios, r.Ratio.(float64))
		}
	}
	if len(ratios) > 0 {
		avg := avgFloat(ratios)
		mx := maxFloat(ratios)
		fmt.Printf("\n平均 thinking/narrative: %.2fx; 最坏: %.2fx; 样本数: %d\n", avg, mx, len(ratios))
	}
}

func avgFloat(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range xs {
		sum += v
	}
	return sum / float64(len(xs))
}

func maxFloat(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, v := range xs[1:] {
		m = max(m, v)
	}
	return m
}

func compare(before, after []storyRow, bLLM, aLLM map[string]*llmSlot) {
	fmt.Println("\n## 对照表(同回合序号)")
	fmt.Println("turn | before ratio | after  ratio | Δratio | before_llm | after_llm | Δllm | before_reason | after_reason | Δreason")
	fmt.Println("-----------------------------------------------------------------------------------------------------------------")
	n := len(before)
	if len(after) > n {
		n = len(after)
	}
	bRatios, aRatios := []float64{}, []float64{}
	for i := 0; i < n; i++ {
		b := storyRow{}
		a := storyRow{}
		if i < len(before) {
			b = before[i]
		}
		if i < len(after) {
			a = after[i]
		}
		bRatioStr := "    -  "
		aRatioStr := "    -  "
		delta := ""
		if b.Ratio != nil {
			bRatioStr = fmt.Sprintf("%7.2fx", b.Ratio.(float64))
			bRatios = append(bRatios, b.Ratio.(float64))
		}
		if a.Ratio != nil {
			aRatioStr = fmt.Sprintf("%7.2fx", a.Ratio.(float64))
			aRatios = append(aRatios, a.Ratio.(float64))
		}
		if b.Ratio != nil && a.Ratio != nil {
			delta = fmt.Sprintf("%+5.2f", a.Ratio.(float64)-b.Ratio.(float64))
		}
		bSlot := bLLM[b.TurnID]
		aSlot := aLLM[a.TurnID]
		deltaLLM, deltaReason := "", ""
		bLLMStr, aLLMStr := "", ""
		bReasonStr, aReasonStr := "", ""
		if bSlot != nil {
			bLLMStr = fmt.Sprintf("%d", bSlot.LLMCalls)
			bReasonStr = fmt.Sprintf("%d", bSlot.ReasoningTokens)
		}
		if aSlot != nil {
			aLLMStr = fmt.Sprintf("%d", aSlot.LLMCalls)
			aReasonStr = fmt.Sprintf("%d", aSlot.ReasoningTokens)
		}
		if bSlot != nil && aSlot != nil {
			deltaLLM = fmt.Sprintf("%+d", aSlot.LLMCalls-bSlot.LLMCalls)
			deltaReason = fmt.Sprintf("%+d", aSlot.ReasoningTokens-bSlot.ReasoningTokens)
		}
		fmt.Printf("%4d | %11s | %11s | %6s | %10s | %9s | %5s | %13s | %12s | %s\n",
			i+1, bRatioStr, aRatioStr, delta,
			bLLMStr, aLLMStr, deltaLLM,
			bReasonStr, aReasonStr, deltaReason)
	}
	if len(bRatios) > 0 && len(aRatios) > 0 {
		bAvg, bMax := avgFloat(bRatios), maxFloat(bRatios)
		aAvg, aMax := avgFloat(aRatios), maxFloat(aRatios)
		fmt.Printf("\n基线 avg=%.2fx / max=%.2fx → 修复后 avg=%.2fx / max=%.2fx\n", bAvg, bMax, aAvg, aMax)
		fmt.Printf("avg Δ=%+.2fx | max Δ=%+.2fx\n", aAvg-bAvg, aMax-bMax)
	}
}

func main() {
	var (
		compareMode bool
		before      string
		after       string
		beforeRuns  multiFlag
		afterRuns   multiFlag
		runs        multiFlag
	)
	flag.BoolVar(&compareMode, "compare", false, "对照基线 vs 修复后")
	flag.StringVar(&before, "before", "", "基线 story jsonl(配合 --compare)")
	flag.StringVar(&after, "after", "", "修复后 story jsonl(配合 --compare)")
	flag.Var(&beforeRuns, "before-runs", "基线 runs jsonl(可多次)")
	flag.Var(&afterRuns, "after-runs", "修复后 runs jsonl(可多次)")
	flag.Var(&runs, "runs", "关联 runs jsonl(单故事模式,可多次)")
	flag.Parse()

	if compareMode {
		if before == "" || after == "" {
			fmt.Fprintln(os.Stderr, "--compare 需要 --before 与 --after")
			os.Exit(2)
		}
		bRows, err := parseStory(before)
		if err != nil {
			fmt.Fprintf(os.Stderr, "before parse: %v\n", err)
			os.Exit(1)
		}
		aRows, err := parseStory(after)
		if err != nil {
			fmt.Fprintf(os.Stderr, "after parse: %v\n", err)
			os.Exit(1)
		}
		bLLM := parseRuns([]string(beforeRuns))
		aLLM := parseRuns([]string(afterRuns))
		printRows("基线: "+filepath.Clean(before), bRows, bLLM)
		printRows("修复后: "+filepath.Clean(after), aRows, aLLM)
		compare(bRows, aRows, bLLM, aLLM)
		return
	}

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "用法: turn_metrics <story.jsonl> [--runs <run.jsonl>...] 或 --compare ...")
		os.Exit(2)
	}
	rows, err := parseStory(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}
	llm := parseRuns([]string(runs))
	printRows(flag.Arg(0), rows, llm)
}

// multiFlag 允许多值 flag
type multiFlag []string

func (m *multiFlag) String() string     { return fmt.Sprintf("%v", []string(*m)) }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }
