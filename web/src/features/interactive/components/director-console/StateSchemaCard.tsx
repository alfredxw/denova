import { useState } from 'react'
import { ChevronRight, Database } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import type { Snapshot } from '../../types'
import { StateSchemaOverview } from './StateSchemaOverview'

// 状态结构机械信息卡：低频管理内容，默认折叠收纳在导演 tab 底部；
// Schema 在开局提交后即冻结，因此只提供只读的机械信息。
export function StateSchemaCard({ snapshot }: { snapshot: Snapshot | null }) {
  const { t } = useTranslation()
  const initialization = snapshot?.state_schema_initialization
  const status = initialization?.status || ''
  const [open, setOpen] = useState(false)

  if (!snapshot?.actor_state_schema && !initialization) return null

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <CollapsibleTrigger className="group flex w-full min-w-0 items-center gap-2.5 px-3 py-3 text-left transition-colors hover:bg-[var(--nova-hover)]" aria-label={t('directorPanel.stateSchema.title')}>
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--director-brass)]">
          <Database className="h-3.5 w-3.5" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="director-console__display block truncate text-sm font-semibold text-[var(--nova-text)]">{t('directorPanel.stateSchema.title')}</span>
          {status ? (
            <span className="mt-0.5 block truncate text-[9px] uppercase tracking-[0.12em] text-[var(--nova-text-faint)]">
              {t(`directorPanel.stateSchema.status.${status}`, { defaultValue: status })}
            </span>
          ) : null}
        </span>
        <ChevronRight className="h-4 w-4 shrink-0 text-[var(--nova-text-faint)] transition-transform group-data-[state=open]:rotate-90" />
      </CollapsibleTrigger>
      <CollapsibleContent className="border-t border-[var(--nova-border)] px-3 py-3">
        <StateSchemaOverview schema={snapshot?.actor_state_schema} initialization={initialization} />
      </CollapsibleContent>
    </Collapsible>
  )
}
