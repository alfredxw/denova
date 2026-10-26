package statistics

// Totals keeps the denominators so latency/cache aggregation is weighted.
// Performance includes model requests only; opaque turn duration is different.
type Totals struct {
	Requests       int64 `json:"requests"`
	ExternalTurns  int64 `json:"external_turns"`
	FailedRequests int64 `json:"failed_requests"`
	UnknownUsage   int64 `json:"unknown_usage"`
	Prompt         int64 `json:"prompt"`
	Cached         int64 `json:"cached"`
	Output         int64 `json:"output"`
	Tokens         int64 `json:"tokens"`
	DurationMS     int64 `json:"duration_ms"`
	TTFTMS         int64 `json:"ttft_ms"`
	TTFTRequests   int64 `json:"ttft_requests"`
}

func (totals *Totals) Add(call Call) {
	if call.Kind == ModelCall {
		totals.Requests++
		totals.DurationMS += call.DurationMS
		if call.Failed {
			totals.FailedRequests++
		}
		if call.TTFTMS != nil {
			totals.TTFTMS += *call.TTFTMS
			totals.TTFTRequests++
		}
	} else {
		totals.ExternalTurns++
	}
	if call.Usage == nil {
		totals.UnknownUsage++
		return
	}
	totals.Prompt += call.Usage.Prompt
	totals.Cached += call.Usage.Cached
	totals.Output += call.Usage.Output
	tokens := call.Usage.Total
	if tokens == 0 {
		tokens = call.Usage.Prompt + call.Usage.Output
	}
	totals.Tokens += tokens
}
