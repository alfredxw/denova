package chat

import "strings"

type assistantOutputSnapshot struct {
	content  string
	thinking string
}

func (r *chatRun) fullAssistantOutputSnapshot() assistantOutputSnapshot {
	if r == nil {
		return assistantOutputSnapshot{}
	}
	return assistantOutputSnapshot{content: r.fullContent.String(), thinking: r.fullThinking.String()}
}

// captureEffectiveAssistantDelta mirrors only newly generated output from the
// append-only display accumulators. A non-prefix transition is an intentional
// local reset (for example a completed plan protocol), so the effective output
// follows that reset without altering already emitted display events.
func (r *chatRun) captureEffectiveAssistantDelta(before assistantOutputSnapshot) {
	if r == nil {
		return
	}
	r.effectiveOutputSet = true
	captureAssistantBuilderDelta(&r.effectiveContent, before.content, r.fullContent.String())
	captureAssistantBuilderDelta(&r.effectiveThinking, before.thinking, r.fullThinking.String())
}

func captureAssistantBuilderDelta(target *strings.Builder, before, after string) {
	if strings.HasPrefix(after, before) {
		target.WriteString(after[len(before):])
		return
	}
	target.Reset()
	target.WriteString(after)
}

func (r *chatRun) effectiveAssistantOutput() (string, string) {
	if r == nil {
		return "", ""
	}
	if r.effectiveOutputSet {
		if r.effectiveContent.Len() == 0 && r.effectiveThinking.Len() == 0 &&
			(r.capturedContent != "" || r.capturedThinking != "") {
			return r.capturedContent, r.capturedThinking
		}
		return r.effectiveContent.String(), r.effectiveThinking.String()
	}
	return r.fullContent.String(), r.fullThinking.String()
}
