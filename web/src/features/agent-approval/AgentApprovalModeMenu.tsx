import type { LucideIcon } from 'lucide-react'
import { Check, ChevronDown, Loader2, PencilLine, ShieldAlert, ShieldQuestion } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { ComposerMenuSubTrigger } from '@/components/Chat/ComposerMenuRow'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
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
  if (!approval.initialized) return null

  const CurrentIcon = modePresentation[approval.mode].icon
  const changeMode = (next: AgentApprovalMode) => {
    if (approval.saving || next === approval.mode) return
    void approval.setMode(next).then((saved) => {
      if (!saved) toast.error(t('agentApproval.input.changeFailed'))
    })
  }

  const modeItems = (
    <>
      {runActive ? (
        <>
          <div className="px-2 py-1.5 text-[11px] leading-4 text-[var(--nova-text-faint)]">
            {t('agentApproval.input.appliesNextTurn')}
          </div>
          <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
        </>
      ) : null}
      {AGENT_APPROVAL_MODES.map((mode) => {
        const { icon: Icon, tone } = modePresentation[mode]
        const selected = approval.mode === mode
        return (
          <DropdownMenuItem
            key={mode}
            disabled={approval.saving}
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
      <DropdownMenuGroup>
        <DropdownMenuSub>
          <ComposerMenuSubTrigger
            icon={approval.saving ? Loader2 : CurrentIcon}
            iconClassName={approval.saving ? 'animate-spin' : modePresentation[approval.mode].tone}
            label={t('agentApproval.input.section')}
            detail={t(`agentApproval.mode.${approval.mode}.label`)}
            detailTone="faint"
            disabled={approval.saving}
            aria-label={`${t('agentApproval.input.section')}: ${t(`agentApproval.mode.${approval.mode}.label`)}`}
          />
          {/* Narrow viewports use a drill-in layer because side-by-side submenus are clipped by the viewport edge. */}
          <DropdownMenuSubContent className="w-80 max-w-[calc(100vw-1rem)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1.5 text-[var(--nova-text)] max-[700px]:[translate:calc(-100%+0.5rem)_0]">
            {modeItems}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      </DropdownMenuGroup>
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
          aria-label={`${t('agentApproval.input.section')}: ${t(`agentApproval.mode.${approval.mode}.label`)}`}
        >
          {approval.saving
            ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
            : <CurrentIcon className={`h-3.5 w-3.5 ${modePresentation[approval.mode].tone}`} />}
          <span className="nova-agent-approval-label truncate">{t(`agentApproval.mode.${approval.mode}.label`)}</span>
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
