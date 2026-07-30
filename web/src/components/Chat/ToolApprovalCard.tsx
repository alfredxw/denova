import { useEffect, useMemo, useState } from 'react'
import { Check, Loader2, ShieldAlert, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AgentAskResolution, ChatMessage } from '@/lib/api'
import { Button } from '@/components/ui/button'
import type { AskInteractionResolver } from './AskInteractionCard'

interface ToolApprovalCardProps {
  message: ChatMessage
  onResolve?: AskInteractionResolver
}

export function ToolApprovalCard({ message, onResolve }: ToolApprovalCardProps) {
  const { t } = useTranslation()
  const interaction = message.ask
  const approval = interaction?.approval
  const [localResolution, setLocalResolution] = useState<AgentAskResolution | null>(null)
  const [submitting, setSubmitting] = useState<'allow-once' | 'deny' | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    setLocalResolution(null)
    setSubmitting(null)
    setError('')
  }, [interaction?.id])

  const status = localResolution?.status || interaction?.status
  const decision = useMemo(() => {
    const answers = localResolution?.answers || interaction?.answers || []
    return answers[0]?.selected_options?.[0]?.id === 'allow-once' ? 'allowed' : 'denied'
  }, [interaction?.answers, localResolution?.answers])

  if (!interaction || interaction.kind !== 'tool_approval' || !approval) return null
  const pending = status === 'pending'

  const resolve = async (option: 'allow-once' | 'deny') => {
    if (!onResolve || submitting) return
    setSubmitting(option)
    setError('')
    try {
      setLocalResolution(await onResolve(message, {
        status: 'answered',
        answers: [{ question_id: 'tool-approval', selected_option_ids: [option] }],
      }))
    } catch {
      setError(t('agentApproval.approval.submitFailed'))
    } finally {
      setSubmitting(null)
    }
  }

  return (
    <div className="flex justify-start">
      <section className="w-full overflow-hidden rounded-lg border border-amber-500/30 bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]" aria-label={t('agentApproval.approval.title')}>
        <header className="flex min-h-11 items-center gap-2 px-3 py-2.5">
          <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-amber-500/25 bg-amber-500/10 text-amber-500">
            <ShieldAlert className="h-4 w-4" />
          </span>
          <span className="min-w-0 flex-1 font-medium text-[var(--nova-text)]">{t('agentApproval.approval.title')}</span>
          <span className={`shrink-0 rounded-full border px-2 py-0.5 text-[11px] ${
            pending
              ? 'border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-400'
              : decision === 'allowed'
                ? 'border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                : 'border-red-500/25 bg-red-500/10 text-red-600 dark:text-red-400'
          }`}>
            {pending ? t('agentApproval.approval.waiting') : decision === 'allowed' ? t('agentApproval.approval.allowed') : t('agentApproval.approval.denied')}
          </span>
        </header>

        <div className="grid gap-3 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3">
          <div className="flex flex-wrap gap-1.5">
            <MetaChip label={t('agentApproval.approval.tool')} value={approval.tool_name} />
            <MetaChip label={t('agentApproval.approval.mode')} value={t(`agentApproval.mode.${knownMode(approval.mode)}.label`)} />
            <MetaChip label="" value={t(`agentApproval.risk.${knownRisk(approval.risk)}`)} warning />
            {approval.cwd && <MetaChip label={t('agentApproval.approval.cwd')} value={approval.cwd} />}
          </div>

          {approval.command && (
            <div>
              <div className="mb-1.5 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('agentApproval.approval.command')}</div>
              <pre className="m-0 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 font-mono text-[12px] leading-5 text-[var(--nova-text)]">{approval.command}</pre>
            </div>
          )}
          {approval.details && (
            <div>
              <div className="mb-1.5 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('agentApproval.approval.details')}</div>
              <pre className="m-0 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 font-mono text-[12px] leading-5 text-[var(--nova-text)]">{approval.details}</pre>
            </div>
          )}

          <div>
            <div className="mb-1 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('agentApproval.approval.reason')}</div>
            <p className="m-0 leading-5 text-[var(--nova-text-muted)]">{approval.reason}</p>
          </div>

          {error && <p role="alert" className="m-0 text-[11px] text-red-500">{error}</p>}
          {pending && (
            <div className="flex flex-wrap justify-end gap-2">
              <Button type="button" size="sm" variant="outline" disabled={!onResolve || Boolean(submitting)} onClick={() => void resolve('deny')}>
                {submitting === 'deny' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <X className="h-3.5 w-3.5" />}
                {t('agentApproval.approval.deny')}
              </Button>
              <Button type="button" size="sm" disabled={!onResolve || Boolean(submitting)} onClick={() => void resolve('allow-once')}>
                {submitting === 'allow-once' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
                {t('agentApproval.approval.allowOnce')}
              </Button>
            </div>
          )}
        </div>
      </section>
    </div>
  )
}

function MetaChip({ label, value, warning = false }: { label: string; value: string; warning?: boolean }) {
  return (
    <span className={`rounded-full border px-2 py-0.5 text-[10px] ${
      warning
        ? 'border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-400'
        : 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)]'
    }`}>
      {label ? `${label}: ` : ''}{value}
    </span>
  )
}

function knownRisk(risk: string) {
  return risk === 'low' || risk === 'medium' || risk === 'critical' ? risk : 'high'
}

function knownMode(mode: string) {
  return mode === 'write' || mode === 'yolo' ? mode : 'ask'
}
