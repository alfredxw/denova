import { useEffect, useMemo, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getAgentRunTrace, getAgentRunTraces } from '@/lib/api'
import type { AgentRunTrace, AgentRunTraceRecord, AgentRunTraceSummary } from '@/lib/api'

interface AgentTracePanelProps {
  disabled?: boolean
}

export function AgentTracePanel({ disabled }: AgentTracePanelProps) {
  const { t } = useTranslation()
  const [runs, setRuns] = useState<AgentRunTraceSummary[]>([])
  const [selectedID, setSelectedID] = useState('')
  const [trace, setTrace] = useState<AgentRunTrace | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const loadRuns = async () => {
    setLoading(true)
    setError('')
    try {
      const list = await getAgentRunTraces(30)
      setRuns(list)
      const nextID = selectedID || list[0]?.id || ''
      setSelectedID(nextID)
      if (nextID) {
        setTrace(await getAgentRunTrace(nextID))
      } else {
        setTrace(null)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadRuns()
  }, [])

  useEffect(() => {
    if (!selectedID) {
      setTrace(null)
      return
    }
    let cancelled = false
    setLoading(true)
    setError('')
    getAgentRunTrace(selectedID)
      .then((next) => {
        if (!cancelled) setTrace(next)
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [selectedID])

  const contextRecords = useMemo(() => trace?.records.filter((record) => record.type === 'context_ledger') || [], [trace])
  const eventRecords = useMemo(() => trace?.records.filter((record) => record.type === 'event') || [], [trace])

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-surface)]">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
        <div className="min-w-0 flex-1 text-xs font-medium text-[var(--nova-text)]">{t('chat.tracePanel.title')}</div>
        <button
          type="button"
          disabled={disabled || loading}
          onClick={() => void loadRuns()}
          className="nova-nav-item rounded p-1 disabled:cursor-not-allowed disabled:opacity-45"
          aria-label={t('chat.tracePanel.refresh')}
          title={t('chat.tracePanel.refresh')}
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>
      <div className="grid min-h-0 flex-1 grid-rows-[160px_1fr]">
        <div className="min-h-0 overflow-y-auto border-b border-[var(--nova-border)] p-2">
          {runs.length === 0 && !loading ? (
            <div className="px-2 py-4 text-xs text-[var(--nova-text-faint)]">{t('chat.tracePanel.empty')}</div>
          ) : (
            <div className="space-y-1">
              {runs.map((run) => (
                <button
                  key={run.id}
                  type="button"
                  onClick={() => setSelectedID(run.id)}
                  className={`w-full rounded-[6px] border px-2 py-1.5 text-left text-[11px] transition-colors ${selectedID === run.id ? 'border-[var(--nova-accent)] bg-[var(--nova-active)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]'}`}
                >
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="min-w-0 flex-1 truncate font-mono">{run.id}</span>
                    <span className="shrink-0">{run.status}</span>
                  </div>
                  <div className="mt-0.5 flex items-center gap-2 text-[10px] text-[var(--nova-text-faint)]">
                    <span>{formatTraceTime(run.created_at)}</span>
                    <span>{t('chat.tracePanel.events', { count: run.events })}</span>
                    <span>{t('chat.tracePanel.contextParts', { count: run.context_parts })}</span>
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>
        <div className="min-h-0 overflow-y-auto p-3">
          {error && <div className="mb-2 rounded border border-[var(--nova-danger)] px-2 py-1.5 text-xs text-[var(--nova-danger)]">{error}</div>}
          {trace ? (
            <div className="space-y-3">
              <TraceSection title={t('chat.tracePanel.contextLedger')} records={contextRecords} empty={t('chat.tracePanel.noContext')} />
              <TraceSection title={t('chat.tracePanel.eventsTitle')} records={eventRecords} empty={t('chat.tracePanel.noEvents')} />
              {trace.truncated && <div className="text-[11px] text-[var(--nova-text-faint)]">{t('chat.tracePanel.truncated')}</div>}
            </div>
          ) : (
            <div className="px-2 py-4 text-xs text-[var(--nova-text-faint)]">{loading ? t('common.loading') : t('chat.tracePanel.selectRun')}</div>
          )}
        </div>
      </div>
    </div>
  )
}

function TraceSection({ title, records, empty }: { title: string; records: AgentRunTraceRecord[]; empty: string }) {
  return (
    <section>
      <h3 className="mb-1 text-[11px] font-semibold text-[var(--nova-text-muted)]">{title}</h3>
      {records.length === 0 ? (
        <div className="text-[11px] text-[var(--nova-text-faint)]">{empty}</div>
      ) : (
        <div className="space-y-1.5">
          {records.map((record, index) => (
            <div key={`${record.type}-${record.created_at}-${index}`} className="rounded-[6px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
              <div className="mb-1 flex items-center gap-2 text-[10px] text-[var(--nova-text-faint)]">
                <span>{record.type}</span>
                <span>{formatTraceTime(record.created_at)}</span>
              </div>
              <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-words font-mono text-[10px] leading-relaxed text-[var(--nova-text-muted)]">{formatTraceData(record.data)}</pre>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

function formatTraceData(data?: Record<string, unknown>) {
  if (!data) return '{}'
  const text = JSON.stringify(data, null, 2)
  return text.length > 4000 ? `${text.slice(0, 4000)}...` : text
}

function formatTraceTime(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}
