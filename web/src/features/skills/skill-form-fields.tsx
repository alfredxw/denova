import type { ElementType, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { VisibleAgentKey } from '@/features/agents/agent-registry'
import { groupSkillAgents, skillAgentOptions } from './skill-utils'

export function SectionTitle({ icon: Icon, title }: { icon: ElementType; title: string }) {
  return (
    <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
      <Icon className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
      {title}
    </div>
  )
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="block">
      <span className="mb-1.5 block text-[11px] font-medium text-[var(--nova-text-muted)]">{label}</span>
      {children}
    </div>
  )
}

export function PreviewRow({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return (
    <div className={`rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 ${wide ? 'md:col-span-2' : ''}`}>
      <div className="text-[10px] uppercase text-[var(--nova-text-faint)]">{label}</div>
      <div className="mt-1 truncate font-mono text-xs text-[var(--nova-text)]" title={value}>{value}</div>
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
    <div className="space-y-3">
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
