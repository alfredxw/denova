package runtime

import "context"

// trimIdleBindings converges the actor registry toward its configured capacity
// without aborting work. Per-binding close fences serialize a concurrent Open
// with the state-atomic CloseIfIdle decision.
func (r *Runtime) trimIdleBindings(protected string) {
	if r == nil || r.config.MaxOpenBindings <= 0 {
		return
	}
	attempted := make(map[string]struct{})
	for {
		r.mu.Lock()
		if r.closed || len(r.harness) <= r.config.MaxOpenBindings {
			r.mu.Unlock()
			return
		}
		candidateKey := ""
		var candidate *Harness
		var oldest uint64
		for key, harness := range r.harness {
			if key == protected || harness == nil || r.closing[key] != nil {
				continue
			}
			if _, seen := attempted[key]; seen {
				continue
			}
			used := r.access[key]
			if candidate == nil || used < oldest {
				candidateKey, candidate, oldest = key, harness, used
			}
		}
		if candidate == nil {
			r.mu.Unlock()
			return
		}
		pending := &closeCall{ready: make(chan struct{}), ref: candidate.binding.Clone()}
		r.closing[candidateKey] = pending
		r.mu.Unlock()

		closed, err := candidate.CloseIfIdle(context.Background())
		r.mu.Lock()
		if closed && r.harness[candidateKey] == candidate {
			delete(r.harness, candidateKey)
			delete(r.access, candidateKey)
		}
		if r.closing[candidateKey] == pending {
			delete(r.closing, candidateKey)
			pending.err = err
			close(pending.ready)
		}
		r.mu.Unlock()
		if err != nil {
			return
		}
		if !closed {
			attempted[candidateKey] = struct{}{}
		}
	}
}
