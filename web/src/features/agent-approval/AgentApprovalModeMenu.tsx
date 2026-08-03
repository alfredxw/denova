import { useEffect, useState } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Check, ChevronDown, Loader2, PencilLine, ShieldAlert, ShieldQuestion } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAgentApprovalMode } from './AgentApprovalProvider'
import { AGENT_APPROVAL_MODES, type AgentApprovalMode } from './modes'
import type { ConversationConfigController } from '@/features/conversation-config/types'

const modePresentation: Record<AgentApprovalMode, { icon: LucideIcon; tone: string }> = {
  ask: { icon: ShieldQuestion, tone: 'text-amber-500' },
  write: { icon: PencilLine, tone: 'text-emerald-500' },
  full_access: { icon: ShieldAlert, tone: 'text-red-500' },
}

interface AgentApprovalModeMenuProps {
  runActive: boolean
  presentation?: 'standalone' | 'submenu'
  conversationConfig?: ConversationConfigController
}

/** Safety-mode selector that can stand alone or live inside a composer options menu. */
export function AgentApprovalModeMenu({ runActive, presentation = 'standalone', conversationConfig }: AgentApprovalModeMenuProps) {
  const { t } = useTranslation()
  const defaultApproval = useAgentApprovalMode()
  const approval = conversationConfig
    ? {
        mode: conversationConfig.snapshot?.approval_mode ?? defaultApproval.mode,
        initialized: conversationConfig.initialized,
        saving: conversationConfig.saving,
        setMode: async (mode: AgentApprovalMode) => conversationConfig.patch({ approval_mode: mode }),
      }
    : defaultApproval
  const [displayedMode, setDisplayedMode] = useState<AgentApprovalMode>(approval.mode)

  // A run snapshots its starting mode. Keep that exact posture visible until
  // settlement even if another settings surface changes the user preference.
  useEffect(() => {
    if (!runActive) setDisplayedMode(approval.mode)
  }, [approval.mode, runActive])

  if (!approval.initialized) return null

  const CurrentIcon = modePresentation[displayedMode].icon
  const changeMode = (next: AgentApprovalMode) => {
    if (runActive || approval.saving || next === approval.mode) return
    void approval.setMode(next).then((saved) => {
      if (!saved) toast.error(t('agentApproval.input.changeFailed'))
    })
  }

  const modeItems = (
    <>
      {runActive ? (
        <>
          <div className="px-2 py-1.5 text-[11px] leading-4 text-[var(--nova-text-faint)]">
            {t('agentApproval.input.changeBlocked')}
          </div>
          <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
        </>
      ) : null}
      {AGENT_APPROVAL_MODES.map((mode) => {
        const { icon: Icon, tone } = modePresentation[mode]
        const selected = displayedMode === mode
        return (
          <DropdownMenuItem
            key={mode}
            disabled={runActive || approval.saving}
            onSelect={() => changeMode(mode)}
            className="cursor-pointer items-start gap-2 px-2 py-2 focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
          >
            <Icon className={`mt-0.5 h-4 w-4 ${tone}`} />
            <span className="min-w-0 flex-1">
              <span className="flex items-center gap-1.5 font-medium text-[var(--nova-text)]">
                {t(`agentApproval.mode.${mode}.label`)}
              </span>
              <span className="mt-0.5 block text-[11px] leading-4 text-[var(--nova-text-faint)]">
                {t(`agentApproval.mode.${mode}.description`)}
              </span>
            </span>
            {selected ? <Check className="mt-0.5 h-3.5 w-3.5 text-[var(--nova-text-muted)]" /> : null}
          </DropdownMenuItem>
        )
      })}
    </>
  )

  if (presentation === 'submenu') {
    return (
      <DropdownMenuSub>
        <DropdownMenuSubTrigger
          disabled={approval.saving}
          className="flex cursor-pointer items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
          aria-label={`${t('agentApproval.input.section')}: ${t(`agentApproval.mode.${displayedMode}.label`)}`}
          title={runActive ? t('agentApproval.input.changeBlocked') : t(`agentApproval.mode.${displayedMode}.description`)}
        >
          {approval.saving
            ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
            : <CurrentIcon className={`h-3.5 w-3.5 ${modePresentation[displayedMode].tone}`} />}
          <span className="min-w-0 flex-1 truncate">{t('agentApproval.input.section')}</span>
          <span className="max-w-28 shrink-0 truncate text-right text-[10px] text-[var(--nova-text-faint)]">
            {t(`agentApproval.mode.${displayedMode}.label`)}
          </span>
        </DropdownMenuSubTrigger>
        {/* Narrow viewports use a drill-in layer because side-by-side submenus are clipped by the viewport edge. */}
        <DropdownMenuSubContent className="w-80 max-w-[calc(100vw-1rem)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1.5 text-[var(--nova-text)] max-[700px]:[translate:calc(-100%+0.5rem)_0]">
          {modeItems}
        </DropdownMenuSubContent>
      </DropdownMenuSub>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={approval.saving}
          className="nova-agent-composer-pill nova-agent-approval-trigger h-8 max-w-40 shrink-0 gap-1.5 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-xs font-medium text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          aria-label={`${t('agentApproval.input.section')}: ${t(`agentApproval.mode.${displayedMode}.label`)}`}
          title={runActive ? t('agentApproval.input.changeBlocked') : t(`agentApproval.mode.${displayedMode}.description`)}
        >
          {approval.saving
            ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
            : <CurrentIcon className={`h-3.5 w-3.5 ${modePresentation[displayedMode].tone}`} />}
          <span className="nova-agent-approval-label truncate">{t(`agentApproval.mode.${displayedMode}.label`)}</span>
          <ChevronDown className="nova-agent-approval-chevron h-3 w-3 text-[var(--nova-text-faint)]" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        side="top"
        className="w-80 max-w-[calc(100vw-1rem)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1.5 text-[var(--nova-text)]"
      >
        {modeItems}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
