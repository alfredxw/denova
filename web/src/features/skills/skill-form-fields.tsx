import { useTranslation } from 'react-i18next'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { VisibleAgentKey } from '@/features/agents/agent-registry'
import { groupSkillAgents, skillAgentOptions, skillCategoryLabel, skillCategoryOptions, writingWorkflowCapability } from './skill-utils'

export function PreviewRow({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return (
    <div className={`rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 ${wide ? 'md:col-span-2' : ''}`}>
      <div className="text-[10px] uppercase text-[var(--nova-text-faint)]">{label}</div>
      <div className="mt-1 truncate font-mono text-xs text-[var(--nova-text)]">{value}</div>
    </div>
  )
}

export function SkillAgentSelector({
  agents,
  onAgentsChange,
}: {
  agents: VisibleAgentKey[]
  onAgentsChange: (value: VisibleAgentKey[]) => void
}) {
  const { t } = useTranslation()
  const agentGroups = groupSkillAgents(skillAgentOptions)
  const toggleAgent = (agent: VisibleAgentKey, checked: boolean) => {
    if (checked) {
      onAgentsChange(agents.includes(agent) ? agents : [...agents, agent])
      return
    }
    onAgentsChange(agents.filter((item) => item !== agent))
  }

  return (
    <div className="flex flex-col gap-3">
      {agentGroups.map((group) => (
        <div key={group.group}>
          <div className="mb-1.5 text-[11px] font-medium text-[var(--nova-text-faint)]">{t(group.group)}</div>
          <div className="grid gap-2 md:grid-cols-2">
            {group.agents.map((agent) => {
              const Icon = agent.icon
              const checked = agents.includes(agent.key)
              return (
                <label
                  key={agent.key}
                  className={`nova-nav-item flex min-h-14 cursor-pointer items-center gap-3 rounded-[var(--nova-radius)] border px-3 py-2 ${checked ? 'is-active border-[var(--nova-border)]' : 'border-transparent bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:border-[var(--nova-border)]'}`}
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={(event) => toggleAgent(agent.key, event.target.checked)}
                    className="h-3.5 w-3.5"
                  />
                  <Icon className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
                  <span className="min-w-0">
                    <span className="block truncate font-medium text-[var(--nova-text)]">{t(agent.titleKey)}</span>
                    <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{t(agent.subtitleKey)}</span>
                  </span>
                </label>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  )
}

export function SkillClassificationFields({
  category,
  capabilities,
  onCategoryChange,
  onCapabilitiesChange,
}: {
  category: string
  capabilities: string[]
  onCategoryChange: (value: string) => void
  onCapabilitiesChange: (value: string[]) => void
}) {
  const { t } = useTranslation()
  const normalizedCategory = category.trim() || 'general'
  const categories = skillCategoryOptions.includes(normalizedCategory as typeof skillCategoryOptions[number])
    ? skillCategoryOptions
    : [...skillCategoryOptions, normalizedCategory]
  const writingWorkflow = capabilities.includes(writingWorkflowCapability)
  const setWritingWorkflow = (checked: boolean) => {
    if (checked) {
      onCapabilitiesChange(Array.from(new Set([...capabilities, writingWorkflowCapability])))
      return
    }
    onCapabilitiesChange(capabilities.filter((capability) => capability !== writingWorkflowCapability))
  }

  return (
    <div className="grid gap-3 md:grid-cols-2">
      <label className="flex min-w-0 flex-col gap-1.5 text-xs text-[var(--nova-text-muted)]">
        <span className="font-medium text-[var(--nova-text)]">{t('skills.category.label')}</span>
        <Select value={normalizedCategory} onValueChange={onCategoryChange}>
          <SelectTrigger aria-label={t('skills.category.label')} className="w-full border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent className="border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]">
            {categories.map((value) => (
              <SelectItem key={value} value={value} className="text-xs">{skillCategoryLabel(value, t)}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('skills.category.hint')}</span>
      </label>

      <div className="flex min-w-0 flex-col gap-1.5">
        <span className="text-xs font-medium text-[var(--nova-text)]">{t('skills.capabilities.label')}</span>
        <label className="flex min-h-9 cursor-pointer items-center justify-between gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
          <span className="min-w-0">
            <span className="block text-xs text-[var(--nova-text)]">{t('skills.capabilities.writingWorkflow')}</span>
            <span className="mt-0.5 block text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('skills.capabilities.writingWorkflowHint')}</span>
          </span>
          <Switch
            checked={writingWorkflow}
            onCheckedChange={setWritingWorkflow}
            aria-label={t('skills.capabilities.writingWorkflow')}
            className="shrink-0"
          />
        </label>
      </div>
    </div>
  )
}
