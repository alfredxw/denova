import { useEffect, useState } from 'react'
import { ChevronRight, Database } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import type { Snapshot } from '../../types'
import { StateSchemaOverview } from './StateSchemaOverview'

// 状态结构机械信息卡：低频管理内容，默认折叠收纳在导演 tab 底部；
// 初始化进行中或失败时自动展开，提醒用户处理。
export function StateSchemaCard({ storyId, snapshot, onRefresh }: { storyId?: string; snapshot: Snapshot | null; onRefresh?: () => void | Promise<unknown> }) {
  const { t } = useTranslation()
  const initialization = snapshot?.state_schema_initialization
  const status = initialization?.status || ''
  const needsAttention = status === 'running' || status === 'failed'
  const [open, setOpen] = useState(needsAttention)

  useEffect(() => {
    if (needsAttention) setOpen(true)
  }, [needsAttention])

  if (!snapshot?.actor_state_schema && !initialization) return null

  const canReview = !snapshot || (snapshot.graph?.branches.length ?? 0) <= 1
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <CollapsibleTrigger className="group flex w-full min-w-0 items-center gap-2.5 px-3 py-3 text-left transition-colors hover:bg-[var(--nova-hover)]" aria-label={t('directorPanel.stateSchema.title')}>
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--director-brass)]">
          <Database className="h-3.5 w-3.5" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="director-console__display block truncate text-sm font-semibold text-[var(--nova-text)]">{t('directorPanel.stateSchema.title')}</span>
          {status ? (
            <span className={`mt-0.5 block truncate text-[9px] uppercase tracking-[0.12em] ${status === 'failed' ? 'text-[var(--nova-danger)]' : 'text-[var(--nova-text-faint)]'}`}>
              {t(`directorPanel.stateSchema.status.${status}`, { defaultValue: status })}
            </span>
          ) : null}
        </span>
        <ChevronRight className="h-4 w-4 shrink-0 text-[var(--nova-text-faint)] transition-transform group-data-[state=open]:rotate-90" />
      </CollapsibleTrigger>
      <CollapsibleContent className="border-t border-[var(--nova-border)] px-3 py-3">
        <StateSchemaOverview storyId={storyId} schema={snapshot?.actor_state_schema} initialization={initialization} canReview={canReview} onRefresh={onRefresh} />
      </CollapsibleContent>
    </Collapsible>
  )
}
