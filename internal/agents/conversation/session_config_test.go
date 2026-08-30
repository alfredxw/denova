package conversation

import "testing"

func TestLegacyConfigManagerSessionIDsRemainReserved(t *testing.T) {
	for _, id := range []string{
		"config-manager-agent",
		"config-manager-agent-automation-resource-daily-review-0123456789ab",
	} {
		if !IsReservedSessionID(id) {
			t.Fatalf("legacy Config Manager session %q must stay hidden from Project Agent sessions", id)
		}
	}
	if IsReservedSessionID("configuration-planning") {
		t.Fatal("ordinary Project Agent session was treated as reserved")
	}
}
