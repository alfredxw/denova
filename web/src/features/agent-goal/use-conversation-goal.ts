import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { fetchConversationGoal, mutateConversationGoal } from './api'
import type { ConversationConfigBinding } from '@/features/conversation-config/types'
import type { ConversationGoal, ConversationGoalAction } from './types'

export function useConversationGoal(binding: ConversationConfigBinding | undefined, executionActive: boolean) {
  const normalized = useMemo(() => {
    if (!binding || (binding.mode !== 'writing' && binding.mode !== 'agent_chat' && binding.mode !== 'interactive')) return undefined
    return {
      ...binding,
      project_id: binding.project_id?.trim(),
      session_id: binding.session_id?.trim(),
      story_id: binding.story_id?.trim(),
      branch_id: binding.branch_id?.trim(),
    }
  }, [binding?.mode, binding?.project_id, binding?.session_id, binding?.story_id, binding?.branch_id])
  const key = normalized
    ? `${normalized.mode}:${normalized.project_id || ''}:${normalized.session_id || ''}:${normalized.story_id || ''}:${normalized.branch_id || ''}`
    : ''
  const [goal, setGoal] = useState<ConversationGoal | null>(null)
  const [loading, setLoading] = useState(Boolean(normalized))
  const [saving, setSaving] = useState(false)
  const generation = useRef(0)
  const savingRef = useRef(false)
  const wasActive = useRef(executionActive)

  const reload = useCallback(async () => {
    const current = ++generation.current
    if (!normalized) {
      setGoal(null)
      setLoading(false)
      return null
    }
    setLoading(true)
    try {
      const next = await fetchConversationGoal(normalized)
      if (generation.current === current) setGoal(next)
      return next
    } catch (reason) {
      if (generation.current === current) setGoal(null)
      console.warn('[conversation-goal] load failed', { binding: normalized, reason })
      return null
    } finally {
      if (generation.current === current) setLoading(false)
    }
  }, [key])

  useEffect(() => {
    setGoal(null)
    void reload()
    return () => { generation.current += 1 }
  }, [reload])

  useEffect(() => {
    const settled = wasActive.current && !executionActive
    wasActive.current = executionActive
    if (settled) void reload()
  }, [executionActive, reload])

  const mutate = useCallback(async (action: ConversationGoalAction, objective?: string) => {
    if (!normalized || savingRef.current) return null
    savingRef.current = true
    setSaving(true)
    try {
      const expectedRevision = goal?.revision || 0
      const next = await mutateConversationGoal(normalized, action, expectedRevision, objective)
      setGoal(next.status === 'completed' || next.status === 'blocked' || next.status === 'active' || next.status === 'paused' ? next : null)
      return next
    } catch (reason) {
      console.warn('[conversation-goal] mutation failed', { binding: normalized, action, reason })
      await reload()
      return null
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }, [goal?.revision, key, reload])

  return {
    goal,
    loading,
    saving,
    reload,
    set: (objective: string) => mutate('set', objective),
    pause: () => mutate('pause'),
    resume: () => mutate('resume'),
    clear: () => mutate('clear'),
  }
}
