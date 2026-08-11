package compaction_test

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/compaction"
	"github.com/alfredxw/denova/agent/compaction/compactiontest"
)

func TestStandardManagerContract(t *testing.T) {
	compactiontest.RunManagerContract(t, func(t testing.TB) agent.CompactionManager {
		manager, err := compaction.Standard(compaction.StandardConfig{Summarizer: compaction.SummarizerFunc{
			Capability: agent.CapabilityIdentity{Kind: "compaction.contract-summary", Version: 1},
			Func: func(context.Context, compaction.SummaryRequest) (compaction.Summary, error) {
				return compaction.Summary{Content: "contract summary", TokenEstimate: 4}, nil
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return manager
	})
}
