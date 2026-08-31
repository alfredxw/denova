import { useEffect, useState } from 'react'
import { Archive, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { SectionedNavigation, type SectionedNavigationGroup } from '@/components/navigation/sectioned-navigation'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { SidebarGroupAction } from '@/components/ui/sidebar'
import { Textarea } from '@/components/ui/textarea'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import type { AgentRuntimeKind, CustomAgentConfig, Settings } from '@/features/settings/types'
import { createAgentCommandID } from '@/lib/api'
import { AGENTS, type AgentViewDefinition, type VisibleAgentKey } from './agent-registry'
import { contractForRuntimeKind, runtimeKindForContract } from './agent-contracts'

export type AgentSelectionID = VisibleAgentKey | `custom:${string}`

const CUSTOM_AGENT_RUNTIME_KINDS: AgentRuntimeKind[] = ['general', 'ide', 'interactive_story', 'image']

export function AgentList({
  active,
  customAgents,
  onSelect,
  onCreate,
}: {
  active: AgentSelectionID
  customAgents: CustomAgentConfig[]
  onSelect: (agent: AgentSelectionID) => void
  onCreate: (runtimeKind: AgentRuntimeKind) => void
}) {
  const { t } = useTranslation()
  const groups = new Map<string, Array<{ id: AgentSelectionID; agent: AgentViewDefinition; title: string; description: string }>>()
  for (const agent of AGENTS) {
    const group = groups.get(agent.groupKey)
    const item = { id: agent.key, agent, title: t(agent.titleKey), description: t(agent.subtitleKey) }
    if (group) group.push(item)
    else groups.set(agent.groupKey, [item])
  }
  for (const customAgent of customAgents) {
    const runtimeKind = runtimeKindForContract(customAgent.contract)
    const base = AGENTS.find((agent) => agent.key === runtimeKind)
    if (!base || !customAgent.id) continue
    const group = groups.get(base.groupKey) ?? []
    group.push({
      id: `custom:${customAgent.id}`,
      agent: base,
      title: customAgent.name || customAgent.id,
      description: customAgent.description || t('agents.custom.contractValue', { agent: t(base.titleKey) }),
    })
    groups.set(base.groupKey, group)
  }

  const navigationGroups: SectionedNavigationGroup<AgentSelectionID>[] = Array.from(groups, ([group, agents]) => {
    const runtimeKind = CUSTOM_AGENT_RUNTIME_KINDS.find((kind) => AGENTS.find((agent) => agent.key === kind)?.groupKey === group)
    const baseAgent = AGENTS.find((agent) => agent.key === runtimeKind)
    const createLabel = baseAgent ? t('agents.custom.createFor', { agent: t(baseAgent.titleKey) }) : ''
    return {
      id: group,
      title: t(group),
      action: runtimeKind ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <SidebarGroupAction type="button" aria-label={createLabel} onClick={() => onCreate(runtimeKind)}>
              <Plus aria-hidden="true" />
            </SidebarGroupAction>
          </TooltipTrigger>
          <TooltipContent side="right">{createLabel}</TooltipContent>
        </Tooltip>
      ) : undefined,
      items: agents.map((item) => ({
        id: item.id,
        title: item.title,
        description: item.description,
        icon: item.agent.icon,
      })),
    }
  })
  return (
    <div className="nova-embedded-sidebar flex h-full min-h-0 flex-col bg-sidebar text-sidebar-foreground">
      <div className="px-2 pt-2 pb-1">
        <Button
          type="button"
          variant="ghost"
          className="h-9 w-full justify-start rounded-md px-2 text-xs text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
          onClick={() => onCreate('ide')}
        >
          <Plus data-icon="inline-start" />
          {t('agents.custom.create')}
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <SectionedNavigation groups={navigationGroups} activeId={active} onSelect={onSelect} />
      </div>
    </div>
  )
}

export function AgentHeader({ agent, customAgent, onArchive }: { agent: AgentViewDefinition; customAgent?: CustomAgentConfig; onArchive?: () => void }) {
  const { t } = useTranslation()
  const Icon = agent.icon
  return (
    <section className="border-b border-[var(--nova-border)] pb-4">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <Icon className="h-4 w-4 text-[var(--nova-text-muted)]" />
        </div>
        <div className="min-w-0">
          <h1 className="truncate text-sm font-semibold">{customAgent?.name || t(agent.titleKey)}</h1>
          <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{customAgent?.description || t(agent.subtitleKey)}</div>
        </div>
        {customAgent && onArchive ? (
          <Button type="button" variant="ghost" size="sm" className="ml-auto text-[var(--nova-text-muted)]" onClick={onArchive}>
            <Archive className="h-3.5 w-3.5" />
            {t('agents.custom.archive')}
          </Button>
        ) : null}
      </div>
      {customAgent ? <div className="mt-3 text-[10px] uppercase tracking-[0.12em] text-[var(--nova-text-faint)]">{t('agents.custom.contractValue', { agent: t(agent.titleKey) })} · {customAgent.id}</div> : null}
    </section>
  )
}

export function CustomAgentIdentitySection({
  agent,
  value,
  onChange,
}: {
  agent: CustomAgentConfig
  value?: CustomAgentConfig
  onChange: (patch: Partial<Pick<CustomAgentConfig, 'name' | 'description'>>) => void
}) {
  const { t } = useTranslation()
  return (
    <section className="grid gap-3 border-b border-[var(--nova-border)] pb-5 sm:grid-cols-2">
      <label className="grid gap-1.5 text-xs">
        <span className="font-medium">{t('agents.custom.name')}</span>
        <Input
          value={value?.name ?? agent.name ?? ''}
          placeholder={agent.name}
          maxLength={80}
          onChange={(event) => onChange({ name: event.target.value })}
        />
      </label>
      <label className="grid gap-1.5 text-xs">
        <span className="font-medium">{t('agents.custom.baseKind')}</span>
        <Input value={t(AGENTS.find((item) => item.key === runtimeKindForContract(agent.contract))?.titleKey ?? 'agents.ide.title')} disabled />
      </label>
      <label className="grid gap-1.5 text-xs sm:col-span-2">
        <span className="font-medium">{t('agents.custom.description')}</span>
        <Textarea
          autoResize
          value={value?.description ?? agent.description ?? ''}
          placeholder={agent.description}
          maxLength={320}
          onChange={(event) => onChange({ description: event.target.value })}
        />
      </label>
    </section>
  )
}

export function CreateCustomAgentDialog({
  open,
  initialRuntimeKind,
  onOpenChange,
  onCreate,
}: {
  open: boolean
  initialRuntimeKind: AgentRuntimeKind
  onOpenChange: (open: boolean) => void
  onCreate: (agent: CustomAgentConfig & { id: string }) => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [runtimeKind, setRuntimeKind] = useState<AgentRuntimeKind>('ide')

  useEffect(() => {
    if (!open) return
    setName('')
    setDescription('')
    setRuntimeKind(initialRuntimeKind)
  }, [initialRuntimeKind, open])

  const create = () => {
    const normalizedName = name.trim()
    if (!normalizedName) return
    const id = `custom-${createAgentCommandID()}`
    onCreate({
      id,
      name: normalizedName,
      description: description.trim() || t('agents.custom.defaultDescription'),
      contract: contractForRuntimeKind(runtimeKind),
      enabled: true,
    })
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="nova-panel border text-[var(--nova-text)]">
        <DialogHeader>
          <DialogTitle>{t('agents.custom.createTitle')}</DialogTitle>
          <DialogDescription>{t('agents.custom.createDescription')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <label className="grid gap-1.5 text-xs">
            <span className="font-medium">{t('agents.custom.baseKind')}</span>
            <Select value={runtimeKind} onValueChange={(value) => setRuntimeKind(value as AgentRuntimeKind)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {CUSTOM_AGENT_RUNTIME_KINDS.map((kind) => {
                  const definition = AGENTS.find((agent) => agent.key === kind)
                  return definition ? <SelectItem key={kind} value={kind}>{t(definition.titleKey)}</SelectItem> : null
                })}
              </SelectContent>
            </Select>
          </label>
          <label className="grid gap-1.5 text-xs">
            <span className="font-medium">{t('agents.custom.name')}</span>
            <Input value={name} maxLength={80} onChange={(event) => setName(event.target.value)} autoFocus />
          </label>
          <label className="grid gap-1.5 text-xs">
            <span className="font-medium">{t('agents.custom.description')}</span>
            <Textarea autoResize value={description} maxLength={320} onChange={(event) => setDescription(event.target.value)} />
          </label>
        </div>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button type="button" disabled={!name.trim()} onClick={create}>{t('agents.custom.create')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function findCustomAgent(settings: Settings, id: string) {
  if (!id) return undefined
  return settings.custom_agents?.find((agent) => agent.id === id)
}

export function updateCustomAgent(
  settings: Settings,
  effective: CustomAgentConfig,
  mutate: (agent: CustomAgentConfig) => CustomAgentConfig,
): Settings {
  const agents = [...(settings.custom_agents ?? [])]
  const index = agents.findIndex((agent) => agent.id === effective.id)
  const current = index >= 0 ? agents[index] : { ...effective }
  const next = mutate(current)
  if (index >= 0) agents[index] = next
  else agents.push(next)
  return { ...settings, custom_agents: agents }
}

export function mergeCustomAgentViews(parent: CustomAgentConfig[] = [], child: CustomAgentConfig[] = []) {
  const values = new Map<string, CustomAgentConfig>()
  for (const agent of parent) {
    if (agent.id) values.set(agent.id, agent)
  }
  for (const agent of child) {
    if (!agent.id) continue
    values.set(agent.id, agent)
  }
  return Array.from(values.values())
}

export function isVisibleCustomAgent(agent: CustomAgentConfig) {
  return Boolean(agent.id && agent.name && runtimeKindForContract(agent.contract) && agent.enabled !== false)
}
