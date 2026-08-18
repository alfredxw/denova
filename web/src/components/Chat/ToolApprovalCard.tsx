import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Check, Loader2, ShieldAlert, ShieldCheck, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AgentAskResolution } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { normalizeAgentApprovalMode } from '@/features/agent-approval/modes'
import type { AskInteractionMessage, AskInteractionResolver } from './AskInteractionCard'

type ApprovalOption = 'allow-once' | 'allow-workspace' | 'deny'

interface ToolApprovalPanelProps {
  message: AskInteractionMessage
  onResolve?: AskInteractionResolver
  embedded?: boolean
  onLayoutChange?: (element: HTMLElement) => void
}

/** Approval state rendered inside the tool call that owns it. */
export function ToolApprovalPanel({ message, onResolve, embedded = false, onLayoutChange }: ToolApprovalPanelProps) {
  const { t } = useTranslation()
  const interaction = message.ask
  const approval = interaction?.approval
  const [localResolution, setLocalResolution] = useState<AgentAskResolution | null>(null)
  const [submitting, setSubmitting] = useState<ApprovalOption | null>(null)
  const [error, setError] = useState('')
  const panelRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    setLocalResolution(null)
    setSubmitting(null)
    setError('')
  }, [interaction?.id])

  useLayoutEffect(() => {
    const status = localResolution?.status || interaction?.status
    if (interaction?.kind === 'tool_approval' && interaction.approval && status === 'pending' && panelRef.current) {
      onLayoutChange?.(panelRef.current)
    }
  }, [interaction?.approval, interaction?.id, interaction?.kind, interaction?.status, localResolution?.status, onLayoutChange])

  if (!interaction || interaction.kind !== 'tool_approval' || !approval) return null
  const status = localResolution?.status || interaction.status
  const selectedOption = (localResolution?.answers || interaction.answers || [])[0]?.selected_options?.[0]?.id
  const pending = status === 'pending'
  const canAlwaysAllow = approval.can_remember === true && Boolean(approval.rule_display_pattern)
  const statusKey = pending
    ? 'waiting'
    : status === 'cancelled'
      ? 'cancelled'
      : selectedOption === 'allow-workspace'
        ? 'allowedWorkspace'
        : selectedOption === 'allow-once'
          ? 'allowed'
          : 'denied'
  const statusTone = pending
    ? 'border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-300'
    : selectedOption === 'allow-once' || selectedOption === 'allow-workspace'
      ? 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
      : 'border-red-500/25 bg-red-500/10 text-red-700 dark:text-red-300'
  const reason = t(approvalReasonKey(approval.rule_id), {
    defaultValue: t('agentApproval.reason.generic'),
  })

  const resolve = async (option: ApprovalOption) => {
    if (!onResolve || submitting) return
    setSubmitting(option)
    setError('')
    try {
      const resolution = await onResolve(message, {
        status: 'answered',
        answers: [{ question_id: 'tool-approval', selected_option_ids: [option] }],
      })
      setLocalResolution(resolution)
      const resolvedOption = resolution.answers?.[0]?.selected_options?.[0]?.id
      if (resolution.status === 'answered' && resolvedOption === 'allow-workspace' && typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'tool-approval' } }))
      }
    } catch {
      setError(t('agentApproval.approval.submitFailed'))
    } finally {
      setSubmitting(null)
    }
  }

  return (
    <section
      ref={panelRef}
      data-tool-approval-panel
      className={`grid gap-2.5 font-sans ${embedded ? 'rounded-md border border-amber-500/25 bg-amber-500/[0.06] p-2.5' : 'p-3'}`}
      aria-label={t('agentApproval.approval.title')}
      aria-live="polite"
    >
      <div className="flex min-w-0 items-center gap-2">
        <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-300">
          <ShieldAlert className="h-3.5 w-3.5" />
        </span>
        <span className="min-w-0 flex-1 text-xs font-medium text-[var(--nova-text)]">{t('agentApproval.approval.title')}</span>
        <span className={`shrink-0 rounded-full border px-2 py-0.5 text-[10px] ${statusTone}`}>
          {t(`agentApproval.approval.${statusKey}`)}
        </span>
      </div>

      {!embedded && (
        <div className="flex flex-wrap gap-1.5">
          <MetaChip label={t('agentApproval.approval.tool')} value={approval.tool_name} />
          <MetaChip label={t('agentApproval.approval.mode')} value={t(`agentApproval.mode.${knownMode(approval.mode)}.label`)} />
          <MetaChip label="" value={t(`agentApproval.risk.${knownRisk(approval.risk)}`)} warning />
          {approval.cwd && <MetaChip label={t('agentApproval.approval.cwd')} value={approval.cwd} />}
        </div>
      )}

      {approval.command && (
        <pre className="m-0 min-w-0 max-w-full max-h-40 overflow-x-hidden overflow-y-auto whitespace-pre-wrap rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 font-mono text-[11px] leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{approval.command}</pre>
      )}
      {!approval.command && approval.details && (
        <pre className="m-0 min-w-0 max-w-full max-h-40 overflow-x-hidden overflow-y-auto whitespace-pre-wrap rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 font-mono text-[11px] leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{approval.details}</pre>
      )}

      <div className="grid gap-1 text-[11px] leading-4 text-[var(--nova-text-muted)]">
        <span><span className="font-medium text-[var(--nova-text-faint)]">{t('agentApproval.approval.reason')}: </span>{reason}</span>
        {canAlwaysAllow && (
          <span className="break-words rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5">
            {t('agentApproval.approval.workspaceRuleScope', { pattern: approval.rule_display_pattern })}
          </span>
        )}
      </div>

      {error && <p role="alert" className="m-0 text-[11px] text-[var(--nova-danger)]">{error}</p>}
      {pending && (
        <div className="flex flex-wrap justify-end gap-2">
          <Button type="button" size="sm" variant="outline" disabled={!onResolve || Boolean(submitting)} onClick={() => void resolve('deny')}>
            {submitting === 'deny' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <X className="h-3.5 w-3.5" />}
            {t('agentApproval.approval.deny')}
          </Button>
          {canAlwaysAllow && (
            <Button type="button" size="sm" variant="outline" disabled={!onResolve || Boolean(submitting)} onClick={() => void resolve('allow-workspace')}>
              {submitting === 'allow-workspace' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <ShieldCheck className="h-3.5 w-3.5" />}
              {t('agentApproval.approval.allowWorkspace')}
            </Button>
          )}
          <Button type="button" size="sm" disabled={!onResolve || Boolean(submitting)} onClick={() => void resolve('allow-once')}>
            {submitting === 'allow-once' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
            {t('agentApproval.approval.allowOnce')}
          </Button>
        </div>
      )}
    </section>
  )
}

// Fail-safe rendering for an approval whose tool call is missing from a
// damaged or partial history page. Normal projections never create this card.
export function ToolApprovalCard({ message, onResolve }: ToolApprovalPanelProps) {
  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-amber-500/30 bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <ToolApprovalPanel message={message} onResolve={onResolve} />
      </div>
    </div>
  )
}

function MetaChip({ label, value, warning = false }: { label: string; value: string; warning?: boolean }) {
  return (
    <span className={`rounded-full border px-2 py-0.5 text-[10px] ${
      warning
        ? 'border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-300'
        : 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)]'
    }`}>
      {label ? `${label}: ` : ''}{value}
    </span>
  )
}

function approvalReasonKey(ruleID: string) {
  const known = new Set([
    'network_access', 'external_mutation', 'unknown_scope', 'shell_environment_override',
    'workspace_unavailable', 'invalid_workspace_boundary', 'bash_parse_failed', 'bash_dynamic_syntax',
    'bash_unlisted_command', 'bash_unclassified', 'pwsh_dynamic_syntax', 'pwsh_parse_failed',
    'pwsh_unlisted_command', 'browser_arguments_unknown', 'browser_remote_mutation',
    'browser_command_unknown', 'browser_action_unknown', 'browser_network_access',
    'external_filesystem_read',
  ])
  return known.has(ruleID) ? `agentApproval.reason.${ruleID}` : 'agentApproval.reason.generic'
}

function knownRisk(risk: string) {
  return risk === 'low' || risk === 'medium' || risk === 'critical' ? risk : 'high'
}

function knownMode(mode: string) {
  return normalizeAgentApprovalMode(mode) || 'ask'
}
