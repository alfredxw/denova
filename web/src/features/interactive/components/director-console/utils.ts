import type { TFunction } from 'i18next'
import type { ChatMessage } from '@/lib/api'
import type { DirectorPlanMetadata, Snapshot, TurnDisplayEvent } from '../../types'
import type { DirectorStatusLike } from './types'

export function extractDirectorDisplayEvents(snapshot: Snapshot | null, status?: DirectorStatusLike) {
  const sourceTurnID = status?.source_turn_id || snapshot?.current_turn?.id || ''
  const sourceTurn = sourceTurnID ? (snapshot?.turns || []).find((turn) => turn.id === sourceTurnID) : snapshot?.current_turn
  const events = sourceTurn?.display_events || snapshot?.current_turn?.display_events || []
  return events.filter(isDirectorDisplayEvent)
}

export function isDirectorDisplayEvent(event: TurnDisplayEvent) {
  if (event.agent_kind === 'interactive_director') return true
  const name = event.name || event.content || ''
  if (!['read_file', 'write_file', 'edit_file'].includes(name)) return false
  return `${event.args || ''}\n${event.result || ''}`.includes('director.md')
}

export function displayEventToChatMessage(event: TurnDisplayEvent, fallbackID: string): ChatMessage {
  return {
    id: event.id || fallbackID,
    role: event.role,
    content: event.content || event.name || '',
    name: event.name || event.content,
    args: event.args || '',
    status: event.status || 'success',
    result: event.result || '',
    created_at: event.created_at,
    run_id: event.run_id,
    agent_kind: event.agent_kind,
    agent_name: event.agent_name,
    root_agent_name: event.root_agent_name,
    run_path: event.run_path,
    subagent: event.subagent,
    subagent_session_id: event.subagent_session_id,
    subagent_type: event.subagent_type,
    sse_hidden_fields: event.sse_hidden_fields,
    sse_hidden_reason: event.sse_hidden_reason,
    sse_display_notice: event.sse_display_notice,
    sse_generated_chars: event.sse_generated_chars,
  }
}

export function directorStatusFallback(status: string, t: TFunction) {
  switch (status) {
    case 'running':
    case 'loading':
      return t('directorPanel.directorChat.running')
    case 'ready':
      return t('directorPanel.directorChat.ready')
    case 'failed':
      return t('directorPanel.directorChat.failed')
    case 'conflict':
      return t('directorPanel.directorChat.conflict')
    case 'waiting_opening':
      return t('directorPanel.directorChat.waitingOpening')
    default:
      return t('directorPanel.directorChat.noRun')
  }
}

export function directorStatusLabel(status: DirectorStatusLike | undefined, loading: boolean | undefined, t: TFunction) {
  if (loading && !status?.status) return t('directorPanel.status.running')
  switch (status?.status) {
    case 'running':
      return t('directorPanel.status.running')
    case 'ready':
      return t('directorPanel.status.ready')
    case 'failed':
      return t('directorPanel.status.failed')
    case 'conflict':
      return t('directorPanel.status.conflict')
    case 'waiting_opening':
      return t('directorPanel.status.waitingOpening')
    default:
      return t('directorPanel.status.idle')
  }
}

export function directorPlanTotals(status?: DirectorStatusLike, metadata?: DirectorPlanMetadata) {
  const docs = Object.values(metadata?.docs || {})
  const totalBytes = status?.doc_bytes ?? docs.reduce((sum, doc) => sum + (doc.bytes || 0), 0)
  const visibleBytes = status?.visible_bytes ?? docs.reduce((sum, doc) => sum + (doc.visible_bytes || 0), 0)
  const planned = status?.planned_docs || docs.length || 1
  const completed = status?.completed_docs ?? (metadata?.last_run?.completed_docs || (metadata?.last_run?.status === 'ready' ? planned : 0))
  return { completed, planned, totalBytes, visibleBytes }
}

export function formatBytes(value: number) {
  return `${new Intl.NumberFormat().format(value)} bytes`
}
export function ruleOutcomeClass(outcome: string) {
  if (outcome.includes('success')) return 'shrink-0 text-[var(--nova-success)]'
  if (outcome.includes('failure') || outcome === 'error') return 'shrink-0 text-[var(--nova-danger)]'
  return 'shrink-0 text-[var(--nova-text-muted)]'
}
export function stateEntries(state?: Record<string, unknown>) {
  if (!state) return []
  return Object.entries(state).filter(([, value]) => value !== undefined && value !== null)
}

export function isMissingDirectorPlanError(err: unknown) {
  const message = err instanceof Error ? err.message.toLowerCase() : String(err || '').toLowerCase()
  return message.includes('director plan not found') ||
    message.includes('http 404') ||
    (message.includes('director.md') && message.includes('no such file or directory'))
}

export function formatShortDate(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString()
}
