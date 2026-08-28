import { useCallback, useEffect, useState } from 'react'
import { Bug, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { debugHarnessState } from '@/lib/api'
import type { HarnessDebugResult, HarnessStateSnapshot } from '@/lib/api'
import { AGENTS, type VisibleAgentKey } from '../agent-registry'

export function HarnessDebugPanel({ snapshot }: { snapshot: HarnessStateSnapshot }) {
  const { t } = useTranslation()
  const [agent, setAgent] = useState<VisibleAgentKey>('general')
  const [result, setResult] = useState<HarnessDebugResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setResult(await debugHarnessState(agent, snapshot.revision))
    } catch (cause) {
      setResult(null)
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setLoading(false)
    }
  }, [agent, snapshot.revision])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="h-full overflow-y-auto p-4 sm:p-5">
      <div className="mx-auto max-w-4xl space-y-4">
        <section className="flex flex-wrap items-start gap-3 border-b border-[var(--nova-border)] pb-4">
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2 text-sm font-semibold text-[var(--nova-text)]">
              <Bug className="size-4" />{t('continualLearning.debug.title')}
            </div>
            <p className="mt-1 text-[11px] leading-5 text-[var(--nova-text-muted)]">{t('continualLearning.debug.description')}</p>
          </div>
          <Select value={agent} onValueChange={(value) => setAgent(value as VisibleAgentKey)}>
            <SelectTrigger size="sm" className="w-48" aria-label={t('continualLearning.agent.target')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {AGENTS.map((definition) => (
                <SelectItem key={definition.key} value={definition.key}>{t(definition.titleKey)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button type="button" size="sm" variant="outline" disabled={loading} onClick={() => void load()}>
            <RefreshCw className={loading ? 'animate-spin' : ''} />{t('continualLearning.debug.refresh')}
          </Button>
        </section>

        <div className="flex flex-wrap gap-2 text-[10px] text-[var(--nova-text-muted)]">
          <Badge variant="outline">{t('continualLearning.debug.draftRevision')}: {snapshot.revision.slice(0, 10)}</Badge>
          <Badge variant="outline">{t('continualLearning.debug.publishedRevision')}: {snapshot.published_revision.slice(0, 10)}</Badge>
          {snapshot.changed && <Badge variant="outline" className="text-amber-500">{t('continualLearning.release.unpublished')}</Badge>}
        </div>

        {error ? <div className="border border-red-500/30 bg-red-500/5 p-3 text-xs text-red-400">{error}</div> : null}
        {result ? (
          <div className="grid gap-3 md:grid-cols-2">
            <DebugGroup
              title={t('continualLearning.debug.prompt')}
              items={result.prompt_resource ? [result.prompt_resource] : []}
              empty={t('continualLearning.debug.none')}
            />
            <DebugGroup
              title={t('continualLearning.debug.contexts')}
              items={result.contexts.map((item) => `${item.resource} · ${item.purpose}`)}
              empty={t('continualLearning.debug.none')}
            />
            <DebugGroup
              title={t('continualLearning.debug.scriptTools')}
              items={result.script_tools.map((item) => item.name)}
              empty={t('continualLearning.debug.none')}
            />
            <DebugGroup
              title={t('continualLearning.debug.subagents')}
              items={result.subagents.map((item) => `${item.name} · ${item.resource}`)}
              empty={t('continualLearning.debug.none')}
            />
            <DebugGroup
              title={t('continualLearning.debug.toolDescriptions')}
              items={result.tool_descriptions}
              empty={t('continualLearning.debug.none')}
            />
          </div>
        ) : null}
      </div>
    </div>
  )
}

function DebugGroup({ title, items, empty }: { title: string; items: string[]; empty: string }) {
  return (
    <section className="min-w-0 border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
      <h3 className="text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">{title}</h3>
      {items.length > 0 ? (
        <ul className="mt-2 space-y-1.5 text-[11px] text-[var(--nova-text)]">
          {items.map((item) => <li key={item} className="break-words font-mono">{item}</li>)}
        </ul>
      ) : <p className="mt-2 text-[11px] text-[var(--nova-text-faint)]">{empty}</p>}
    </section>
  )
}
