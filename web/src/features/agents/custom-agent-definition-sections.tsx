import { useMemo, useState, type ReactNode } from 'react'
import { Bot, ChevronDown, ChevronRight, Database, Pin, Plus, ScrollText, Shield, Trash2, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type {
  AgentContextBinding,
  AgentDelegationPolicy,
  AgentSkillPolicy,
  CustomAgentConfig,
  ResolvedAgentDefinition,
  SubAgentConfig,
} from '@/features/settings/types'
import type { SkillSummary } from '@/lib/api'
import { SectionTitle } from './agent-form-controls'
import type { AgentToolDefinition } from './agent-registry'

export function CustomAgentBehaviorSection({ instructions, runtimeContract, outputProtocol, onChange }: {
  instructions: string
  runtimeContract?: string
  outputProtocol?: string
  onChange: (instructions: string) => void
}) {
  const { t } = useTranslation()
  return (
    <section className="flex flex-col gap-3 pb-5">
      <SectionTitle icon={ScrollText} title={t('agents.section.behavior')} />
      <ReadonlyContractBlock title={t('agents.prompt.source.runtime_contract')} content={runtimeContract} />
      <ReadonlyContractBlock title={t('agents.prompt.source.output_protocol')} content={outputProtocol} />
      <label className="grid gap-1.5 text-xs">
        <span className="font-medium text-[var(--nova-text)]">{t('agents.custom.instructions')}</span>
        <span className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agents.custom.instructionsDescription')}</span>
        <Textarea
          autoResize
          value={instructions}
          onChange={(event) => onChange(event.target.value)}
          placeholder={t('agents.custom.instructionsPlaceholder')}
          className="min-h-64 resize-y font-mono text-xs leading-5"
        />
      </label>
    </section>
  )
}

function ReadonlyContractBlock({ title, content = '' }: { title: string; content?: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      <button type="button" onClick={() => setOpen((value) => !value)} className="flex w-full items-center gap-2 px-3 py-2 text-left" aria-expanded={open}>
        {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        <span className="flex-1 text-[11px] font-medium">{title}</span>
        <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{t('agents.prompt.badge.readonly')}</span>
      </button>
      {open ? <pre className="max-h-64 overflow-auto whitespace-pre-wrap border-t border-[var(--nova-border)] p-3 text-[11px] leading-5 text-[var(--nova-text-faint)]">{content || '—'}</pre> : null}
    </div>
  )
}

export function AgentToolGuidanceSection({ rows, value, onChange }: {
  rows: AgentToolDefinition[]
  value: Record<string, string>
  onChange: (value: Record<string, string>) => void
}) {
  const { t } = useTranslation()
  const configuredNames = Object.keys(value)
  const knownToolNames = useMemo(() => Array.from(new Set(rows.filter((row) => row.allowed).flatMap((row) => row.toolNames))).sort(), [rows])
  const [selected, setSelected] = useState('')
  const [exactName, setExactName] = useState('')
  const toolNames = Array.from(new Set([...knownToolNames, ...configuredNames, ...(selected ? [selected] : [])])).sort()
  const active = toolNames.includes(selected) ? selected : toolNames[0] || ''
  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Wrench} title={t('agents.section.toolGuidance')} />
      <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agents.toolGuidance.note')}</p>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input value={exactName} onChange={(event) => setExactName(event.target.value)} maxLength={128} placeholder={t('agents.toolGuidance.exactPlaceholder')} />
        <Button type="button" variant="outline" disabled={!exactName.trim()} onClick={() => { setSelected(exactName.trim()); setExactName('') }}><Plus />{t('agents.toolGuidance.addExact')}</Button>
      </div>
      {active ? (
        <div className="grid gap-3 md:grid-cols-[minmax(180px,0.35fr)_minmax(0,1fr)]">
          <Select value={active} onValueChange={setSelected}>
            <SelectTrigger aria-label={t('agents.toolGuidance.tool')}><SelectValue /></SelectTrigger>
            <SelectContent>{toolNames.map((name) => <SelectItem key={name} value={name}>{name}</SelectItem>)}</SelectContent>
          </Select>
          <Textarea
            autoResize
            value={value[active] ?? ''}
            onChange={(event) => {
              const next = { ...value }
              const guidance = event.target.value
              if (guidance.trim()) next[active] = guidance
              else delete next[active]
              onChange(next)
            }}
            placeholder={t('agents.toolGuidance.placeholder')}
            className="min-h-28 resize-y text-xs leading-5"
          />
        </div>
      ) : <EmptyHint>{t('agents.toolGuidance.empty')}</EmptyHint>}
    </section>
  )
}

export function AgentSkillPolicySection({ skills, value, onChange, allowExplicit = true }: {
  skills: SkillSummary[]
  value: AgentSkillPolicy
  onChange: (value: AgentSkillPolicy) => void
  allowExplicit?: boolean
}) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const mode = value.mode ?? 'managed'
  const pinned = value.pinned ?? []
  const blocked = value.blocked ?? []
  const needle = query.trim().toLowerCase()
  const results = needle ? skills.filter((skill) => `${skill.name}\n${skill.description}`.toLowerCase().includes(needle)).slice(0, 20) : []
  const setState = (name: string, state: 'pinned' | 'blocked' | 'default') => {
    onChange({
      mode,
      pinned: state === 'pinned' ? unique([...pinned.filter((item) => item !== name), name]) : pinned.filter((item) => item !== name),
      blocked: state === 'blocked' ? unique([...blocked.filter((item) => item !== name), name]) : blocked.filter((item) => item !== name),
    })
  }
  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Pin} title={t('agents.section.skills')} />
      <div className="grid gap-3 sm:grid-cols-[220px_minmax(0,1fr)]">
        <Select value={mode} onValueChange={(next) => onChange({ ...value, mode: next as AgentSkillPolicy['mode'] })}>
          <SelectTrigger aria-label={t('agents.skills.policy')}><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="managed">{t('agents.skills.policyManaged')}</SelectItem>
            {allowExplicit ? <SelectItem value="explicit">{t('agents.skills.policyExplicit')}</SelectItem> : null}
          </SelectContent>
        </Select>
        <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('agents.skills.searchPlaceholder')} />
      </div>
      <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agents.skills.policyNote')}</p>
      <div className="flex flex-wrap gap-1.5">
        {pinned.map((name) => <PolicyChip key={`p:${name}`} label={name} tone="positive" onRemove={() => setState(name, 'default')} />)}
        {blocked.map((name) => <PolicyChip key={`b:${name}`} label={name} tone="negative" onRemove={() => setState(name, 'default')} />)}
        {!pinned.length && !blocked.length ? <span className="text-[11px] text-[var(--nova-text-faint)]">{t('agents.skills.noExceptions')}</span> : null}
      </div>
      {results.length ? <div className="grid gap-2 lg:grid-cols-2">{results.map((skill) => (
        <div key={`${skill.scope}:${skill.name}`} className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2">
          <div className="min-w-0 flex-1"><div className="truncate font-mono text-xs">/{skill.name}</div><div className="truncate text-[10px] text-[var(--nova-text-faint)]">{skill.description}</div></div>
          <Button type="button" size="xs" variant={pinned.includes(skill.name) ? 'secondary' : 'ghost'} onClick={() => setState(skill.name, 'pinned')}>{t('agents.skills.pin')}</Button>
          <Button type="button" size="xs" variant={blocked.includes(skill.name) ? 'destructive' : 'ghost'} onClick={() => setState(skill.name, 'blocked')}>{t('agents.skills.block')}</Button>
        </div>
      ))}</div> : null}
    </section>
  )
}

function PolicyChip({ label, tone, onRemove }: { label: string; tone: 'positive' | 'negative'; onRemove: () => void }) {
  return <button type="button" onClick={onRemove} className={`rounded border px-2 py-1 font-mono text-[10px] ${tone === 'positive' ? 'border-[var(--nova-success)]/30 text-[var(--nova-success)]' : 'border-[var(--nova-danger)]/30 text-[var(--nova-danger)]'}`}>/{label} ×</button>
}

export function AgentDelegationPolicySection({ value, runtimeKind, subAgents, onChange }: {
  value: AgentDelegationPolicy
  runtimeKind: string
  subAgents: SubAgentConfig[]
  onChange: (value: AgentDelegationPolicy) => void
}) {
  const { t } = useTranslation()
  const mode = value.mode ?? 'compatible'
  const selected = value.agent_ids ?? []
  const candidates = [{ id: 'general-purpose', name: t('agents.subAgents.general.title'), description: t('agents.subAgents.general.description') }, ...subAgents
    .filter((item) => item.enabled !== false && item.id && item.parents?.includes(runtimeKind))
    .map((item) => ({ id: item.id!, name: item.name || item.id!, description: item.description || '' }))]
  return (
    <section className="flex flex-col gap-3 pb-5">
      <SectionTitle icon={Bot} title={t('agents.section.delegation')} />
      <Select value={mode} onValueChange={(next) => onChange({ ...value, mode: next as AgentDelegationPolicy['mode'] })}>
        <SelectTrigger className="max-w-xs" aria-label={t('agents.delegation.policy')}><SelectValue /></SelectTrigger>
        <SelectContent>
          <SelectItem value="compatible">{t('agents.delegation.compatible')}</SelectItem>
          <SelectItem value="selected">{t('agents.delegation.selected')}</SelectItem>
          <SelectItem value="disabled">{t('agents.delegation.disabled')}</SelectItem>
        </SelectContent>
      </Select>
      <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agents.delegation.note')}</p>
      {mode === 'selected' ? <div className="grid gap-2 lg:grid-cols-2">{candidates.map((candidate) => {
        const checked = selected.includes(candidate.id)
        return <label key={candidate.id} className="flex items-center gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2">
          <div className="min-w-0 flex-1"><div className="truncate text-xs font-medium">{candidate.name}</div><div className="truncate text-[10px] text-[var(--nova-text-faint)]">{candidate.description}</div></div>
          <Switch checked={checked} onCheckedChange={(enabled) => onChange({ mode: 'selected', agent_ids: enabled ? unique([...selected, candidate.id]) : selected.filter((id) => id !== candidate.id) })} />
        </label>
      })}</div> : null}
    </section>
  )
}

export function AgentContextBindingsSection({ value, onChange }: {
  value: AgentContextBinding[]
  onChange: (value: AgentContextBinding[]) => void
}) {
  const { t } = useTranslation()
  const [openID, setOpenID] = useState('')
  const add = () => {
    let index = value.length + 1
    while (value.some((item) => item.id === `context-${index}`)) index++
    const next = { id: `context-${index}`, name: t('agents.contextBindings.newName'), purpose: 'apply user-authored Agent context', slot: 'stable' as const, content: '', hard_limit_bytes: 262144 }
    onChange([...value, next])
    setOpenID(next.id)
  }
  const update = (id: string, patch: Partial<AgentContextBinding>) => onChange(value.map((item) => item.id === id ? { ...item, ...patch } : item))
  return (
    <section className="flex flex-col gap-3 pb-5">
      <div className="flex items-center gap-2"><SectionTitle icon={Database} title={t('agents.section.contextBindings')} /><Button type="button" size="xs" variant="ghost" className="ml-auto" onClick={add}><Plus />{t('agents.contextBindings.add')}</Button></div>
      <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agents.contextBindings.note')}</p>
      {!value.length ? <EmptyHint>{t('agents.contextBindings.empty')}</EmptyHint> : value.map((binding) => {
        const open = openID === binding.id
        return <div key={binding.id} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
          <button type="button" onClick={() => setOpenID(open ? '' : binding.id)} className="flex w-full items-center gap-2 px-3 py-2 text-left">
            {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
            <span className="min-w-0 flex-1 truncate text-xs font-medium">{binding.name || binding.id}</span>
            <span className="text-[10px] uppercase text-[var(--nova-text-faint)]">{t(`agents.contextBindings.slot.${binding.slot || 'stable'}`)}</span>
          </button>
          {open ? <div className="grid gap-3 border-t border-[var(--nova-border)] p-3 sm:grid-cols-2">
            <Input value={binding.name ?? ''} aria-label={t('agents.contextBindings.name')} placeholder={t('agents.contextBindings.name')} onChange={(event) => update(binding.id, { name: event.target.value })} />
            <Select value={binding.slot ?? 'stable'} onValueChange={(slot) => update(binding.id, { slot: slot as AgentContextBinding['slot'] })}><SelectTrigger aria-label={t('agents.contextBindings.slot')}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="stable">{t('agents.contextBindings.slot.stable')}</SelectItem><SelectItem value="session">{t('agents.contextBindings.slot.session')}</SelectItem><SelectItem value="turn">{t('agents.contextBindings.slot.turn')}</SelectItem></SelectContent></Select>
            <Input value={binding.purpose ?? ''} aria-label={t('agents.contextBindings.purpose')} placeholder={t('agents.contextBindings.purpose')} onChange={(event) => update(binding.id, { purpose: event.target.value })} className="sm:col-span-2" />
            <Textarea autoResize value={binding.content} aria-label={t('agents.contextBindings.content')} placeholder={t('agents.contextBindings.content')} onChange={(event) => update(binding.id, { content: event.target.value })} className="min-h-36 resize-y font-mono text-xs sm:col-span-2" />
            <label className="flex items-center gap-2 text-[11px] text-[var(--nova-text-faint)]">{t('agents.contextBindings.hardLimit')}<Input type="number" min={1024} max={16777216} value={binding.hard_limit_bytes ?? 262144} onChange={(event) => update(binding.id, { hard_limit_bytes: Number(event.target.value) || 262144 })} /></label>
            <Button type="button" variant="ghost" className="justify-self-end text-[var(--nova-danger)]" onClick={() => onChange(value.filter((item) => item.id !== binding.id))}><Trash2 />{t('common.delete')}</Button>
          </div> : null}
        </div>
      })}
    </section>
  )
}

export function ResolvedAgentSection({ agent, resolved, tools }: {
  agent: CustomAgentConfig
  resolved?: ResolvedAgentDefinition
  tools?: AgentToolDefinition[]
}) {
  const { t } = useTranslation()
  const allowedTools = Array.from(new Set(tools?.filter((item) => item.allowed).flatMap((item) => item.toolNames) ?? [])).sort()
  const policy = agent.skill_policy ?? resolved?.skill_policy ?? {}
  const bindings = agent.context_bindings ?? resolved?.context_bindings ?? []
  return (
    <section className="flex flex-col gap-3 pb-5">
      <SectionTitle icon={Shield} title={t('agents.section.resolved')} />
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        <ResolvedCard label={t('agents.resolved.contract')} value={resolved?.contract || agent.contract || '—'} />
        <ResolvedCard label={t('agents.resolved.revision')} value={(resolved?.revision || '—').slice(0, 12)} mono />
        <ResolvedCard label={t('agents.resolved.instructions')} value={t('agents.resolved.bytes', { count: new Blob([agent.instructions ?? resolved?.instructions ?? '']).size })} />
        <ResolvedCard label={t('agents.resolved.tools')} value={String(allowedTools.length)} />
        <ResolvedCard label={t('agents.resolved.skills')} value={`${policy.mode ?? 'managed'} · +${policy.pinned?.length ?? 0} / −${policy.blocked?.length ?? 0}`} />
        <ResolvedCard label={t('agents.resolved.context')} value={String(bindings.length)} />
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
        <div className="mb-2 text-[10px] uppercase tracking-[0.12em] text-[var(--nova-text-faint)]">{t('agents.resolved.toolNames')}</div>
        <div className="flex flex-wrap gap-1.5">{allowedTools.length ? allowedTools.map((name) => <span key={name} className="rounded border border-[var(--nova-border)] px-1.5 py-0.5 font-mono text-[10px]">{name}</span>) : '—'}</div>
      </div>
    </section>
  )
}

function ResolvedCard({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2"><div className="text-[10px] uppercase text-[var(--nova-text-faint)]">{label}</div><div className={`mt-1 truncate text-xs ${mono ? 'font-mono' : ''}`}>{value}</div></div>
}

function EmptyHint({ children }: { children: ReactNode }) {
  return <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3 text-[11px] text-[var(--nova-text-faint)]">{children}</div>
}

function unique(values: string[]) { return Array.from(new Set(values)) }
