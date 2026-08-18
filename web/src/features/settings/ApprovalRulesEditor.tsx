import { Loader2, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import type { AgentApprovalRule } from './types'

interface ApprovalRulesEditorProps {
  rules?: AgentApprovalRule[]
  revokingRuleID?: string
  onRevoke: (id: string) => void
}

export function ApprovalRulesEditor({ rules = [], revokingRuleID = '', onRevoke }: ApprovalRulesEditorProps) {
  const { t, i18n } = useTranslation()
  return (
    <SettingsFieldRow title={t('agentApproval.rules.title')} description={t('agentApproval.rules.description')} controlClassName="sm:max-w-xl sm:flex-1">
      <div className="grid w-full gap-2">
        {rules.length === 0 ? (
          <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3 text-xs text-[var(--nova-text-faint)]">
            {t('agentApproval.rules.empty')}
          </div>
        ) : (
          <div className="grid gap-2">
            {rules.map((rule) => (
              <div key={rule.id} className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2.5">
                <div className="grid min-w-0 gap-1">
                  <code className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-xs text-[var(--nova-text)]">
                    {rule.display_pattern || rule.approved_input}
                  </code>
                  <div className="flex min-w-0 flex-wrap gap-x-3 gap-y-0.5 text-[10px] leading-4 text-[var(--nova-text-faint)]">
                    <span className="min-w-0 truncate">{t('agentApproval.rules.workspace')}: {rule.workspace || rule.project_id}</span>
                    <span>{rule.tool_name}</span>
                    <span>{t('agentApproval.rules.created')}: {formatRuleDate(rule.created_at, i18n.resolvedLanguage)}</span>
                  </div>
                </div>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  className="text-[var(--nova-text-muted)] hover:text-[var(--nova-danger)]"
                  aria-label={t('agentApproval.rules.remove')}
                  disabled={Boolean(revokingRuleID)}
                  onClick={() => onRevoke(rule.id)}
                >
                  {revokingRuleID === rule.id
                    ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    : <Trash2 className="h-3.5 w-3.5" />}
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
    </SettingsFieldRow>
  )
}

function formatRuleDate(value: string, locale?: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(locale || undefined, {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  }).format(date)
}
