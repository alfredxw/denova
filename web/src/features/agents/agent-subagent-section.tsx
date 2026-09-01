import { useMemo, useState } from 'react'
import { Bot, Edit3, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type { AgentModelOverride, AgentToolOverride, LayeredSettings, Settings, SettingsLayer, SubAgentConfig } from '@/features/settings/types'
import { THINKING_LEVELS } from '@/features/settings/thinking-levels'
import { Field, SectionTitle, SwitchWithInheritance, ToggleSwitch } from './agent-form-controls'
import { SUB_AGENT_PARENT_KEYS } from './agent-registry'
import type { AgentToolDefinition, SubAgentParentKey, ToolKey, VisibleAgentKey } from './agent-registry'

export function AgentSubAgentSection({ agent, toolRows, generalSettings, effectiveGeneralSettings, subAgents, effectiveSubAgents, profiles, onGeneralChange, onChange }: {
  agent: SubAgentParentKey
  toolRows: AgentToolDefinition[]
  generalSettings: Settings['general_sub_agents']
  effectiveGeneralSettings: Settings['general_sub_agents']
  subAgents: SubAgentConfig[]
  effectiveSubAgents: SubAgentConfig[]
  profiles: Array<{ id: string; label: string }>
  onGeneralChange: (agent: SubAgentParentKey, value: boolean | null) => void
  onChange: (updater: (current: SubAgentConfig[]) => SubAgentConfig[]) => void
}) {
  const { t } = useTranslation()
  const [deleteTarget, setDeleteTarget] = useState<SubAgentConfig | null>(null)
  const [editingSubAgent, setEditingSubAgent] = useState<{ id: string; value: SubAgentConfig } | null>(null)
  const availableToolRows = toolRows.filter((tool) => tool.availableToSubAgents)
  const visibleSubAgents = useMemo(() => mergeVisibleSubAgents(effectiveSubAgents, subAgents)
    .filter((subAgent) => effectiveSubAgentParents(subAgent).includes(agent)), [agent, effectiveSubAgents, subAgents])
  const generalExplicit = generalSettings?.[agent]
  const generalEnabled = resolveGeneralSubAgentEnabled(effectiveGeneralSettings, agent)
  const addSubAgent = () => {
    const nextID = nextSubAgentID(mergeVisibleSubAgents(effectiveSubAgents, subAgents))
    setEditingSubAgent({
      id: nextID,
      value: {
        id: nextID,
        name: t('agents.subAgents.newName'),
        description: t('agents.subAgents.newDescription'),
        system_prompt: t('agents.subAgents.newPrompt'),
        enabled: true,
        parents: [agent],
        model: {},
        tools: {},
      },
    })
  }
  const updateSubAgent = (id: string, patch: Partial<SubAgentConfig>) => {
    const base = visibleSubAgents.find((subAgent) => normalizeSubAgentID(subAgent.id || '') === id)
    if (!base) return
    onChange((current) => upsertSubAgentOverride(current, normalizeSubAgentConfig({ ...base, ...patch }), id))
  }
  const setSubAgentAvailableForCurrent = (subAgent: SubAgentConfig, available: boolean) => {
    const id = normalizeSubAgentID(subAgent.id || '')
    if (!id) return
    const currentParents = effectiveSubAgentParents(subAgent)
    const nextParents = available
      ? SUB_AGENT_PARENT_KEYS.filter((parent) => parent === agent || currentParents.includes(parent))
      : currentParents.filter((parent) => parent !== agent)
    updateSubAgent(id, { parents: nextParents, enabled: true })
  }
  const editSubAgent = (subAgent: SubAgentConfig) => {
    const id = normalizeSubAgentID(subAgent.id || '')
    if (!id) return
    setEditingSubAgent({ id, value: normalizeSubAgentConfig(subAgent) })
  }
  const updateEditingSubAgent = (id: string, patch: Partial<SubAgentConfig>) => {
    setEditingSubAgent((current) => {
      if (!current || current.id !== id) return current
      return { ...current, value: normalizeSubAgentConfig({ ...current.value, ...patch }) }
    })
  }
  const finishEditingSubAgent = () => {
    if (!editingSubAgent) return
    const next = normalizeSubAgentConfig(editingSubAgent.value)
    onChange((current) => upsertSubAgentOverride(current, next, editingSubAgent.id))
    setEditingSubAgent(null)
  }
  const deleteSubAgentForCurrentParent = () => {
    if (!deleteTarget) return
    const deleteID = normalizeSubAgentID(deleteTarget.id || '')
    if (!deleteID) return
    const base = visibleSubAgents.find((subAgent) => normalizeSubAgentID(subAgent.id || '') === deleteID) ?? deleteTarget
    onChange((current) => {
      return upsertSubAgentOverride(current, normalizeSubAgentConfig({ ...base, enabled: true, parents: subAgentParentsWithout(base, agent) }), deleteID)
    })
    if (editingSubAgent?.id === deleteID) setEditingSubAgent(null)
    setDeleteTarget(null)
  }
  const deleteSubAgentEverywhere = () => {
    if (!deleteTarget) return
    const deleteID = normalizeSubAgentID(deleteTarget.id || '')
    if (!deleteID) return
    const base = visibleSubAgents.find((subAgent) => normalizeSubAgentID(subAgent.id || '') === deleteID) ?? deleteTarget
    onChange((current) => {
      const currentHasID = current.some((subAgent) => normalizeSubAgentID(subAgent.id || '') === deleteID)
      if (!currentHasID) {
        return upsertSubAgentOverride(current, normalizeSubAgentConfig({ ...base, enabled: false, parents: [] }), deleteID)
      }
      return current.filter((subAgent) => normalizeSubAgentID(subAgent.id || '') !== deleteID)
    })
    if (editingSubAgent?.id === deleteID) setEditingSubAgent(null)
    setDeleteTarget(null)
  }

  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <SectionTitle icon={Bot} title={t('agents.section.subAgents')} />
        <button
          type="button"
          onClick={addSubAgent}
          className="nova-nav-item ml-auto inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1 text-[11px] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('agents.subAgents.add')}
        </button>
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <Bot className="mt-0.5 h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <span className="font-medium">{t('agents.subAgents.general.title')}</span>
              <span className={`rounded-[var(--nova-radius)] border px-1.5 py-0.5 text-[10px] ${generalEnabled ? 'border-[var(--nova-success)]/30 bg-[var(--nova-success-bg)] text-[var(--nova-success)]' : 'border-[var(--nova-danger)]/30 bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]'}`}>
                {generalEnabled ? t('agents.option.on') : t('agents.option.off')}
              </span>
            </div>
            <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agents.subAgents.general.description')}</div>
          </div>
          <SwitchWithInheritance
            checked={Boolean(generalEnabled)}
            onChange={(checked) => onGeneralChange(agent, checked)}
            ariaLabel={t('agents.subAgents.general.enabled')}
            inherited={generalExplicit === undefined || generalExplicit === null}
            onReset={generalExplicit !== undefined && generalExplicit !== null ? () => onGeneralChange(agent, null) : undefined}
          />
        </div>
      </div>
      {visibleSubAgents.length === 0 ? (
        <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3 text-[11px] text-[var(--nova-text-faint)]">
          {t('agents.subAgents.empty')}
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {visibleSubAgents.map((subAgent, index) => (
            <SubAgentRow
              key={`${subAgent.id || 'subagent'}:${index}`}
              agent={agent}
              subAgent={subAgent}
              onToggle={(enabled) => setSubAgentAvailableForCurrent(subAgent, enabled)}
              onEdit={() => editSubAgent(subAgent)}
              onDelete={() => setDeleteTarget(subAgent)}
            />
          ))}
        </div>
      )}
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('agents.subAgents.note')}
      </div>
      <Dialog open={Boolean(editingSubAgent)} onOpenChange={(open) => { if (!open) setEditingSubAgent(null) }}>
        {editingSubAgent && (
          <DialogContent
            className="nova-panel flex max-h-[min(760px,calc(100vh-2rem))] flex-col overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
          >
            <DialogHeader className="shrink-0 gap-1 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-4 py-3 text-left">
              <DialogTitle className="text-sm">{editingSubAgent.value.name || editingSubAgent.value.id || t('agents.subAgents.untitled')}</DialogTitle>
              <DialogDescription className="text-[11px] leading-5 text-[var(--nova-text-faint)]">
                {t('agents.subAgents.dialogDescription')}
              </DialogDescription>
            </DialogHeader>
            <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
              <SubAgentEditor
                id={editingSubAgent.id}
                agent={agent}
                subAgent={editingSubAgent.value}
                toolRows={availableToolRows}
                profiles={profiles}
                onChange={updateEditingSubAgent}
              />
            </div>
            <DialogFooter className="mx-0 mb-0 shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-4 py-3">
              <button type="button" onClick={finishEditingSubAgent} className="nova-nav-item rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-1.5 text-xs text-[var(--nova-text)] hover:bg-[var(--nova-hover)]">
                {t('agents.subAgents.done')}
              </button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>
      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
        <AlertDialogContent size="sm" className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
          <AlertDialogHeader>
            <AlertDialogTitle>{t('agents.subAgents.deleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('agents.subAgents.deleteDescription')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <button type="button" onClick={deleteSubAgentForCurrentParent} className="nova-nav-item rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1.5 text-xs text-[var(--nova-text)] hover:bg-[var(--nova-hover)]">
              {t('agents.subAgents.deleteCurrentParent')}
            </button>
            <AlertDialogAction variant="destructive" onClick={deleteSubAgentEverywhere}>{t('agents.subAgents.deleteEverywhere')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}

function SubAgentRow({ agent, subAgent, onToggle, onEdit, onDelete }: {
  agent: SubAgentParentKey
  subAgent: SubAgentConfig
  onToggle: (enabled: boolean) => void
  onEdit: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const parents = effectiveSubAgentParents(subAgent)
  const availableForCurrent = parents.includes(agent)
  const enabled = (subAgent.enabled ?? true) && availableForCurrent
  return (
    <div data-subagent-id={subAgent.id} className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
      <Bot className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="min-w-0 truncate font-medium">{subAgent.name || subAgent.id || t('agents.subAgents.untitled')}</span>
          <span className={`shrink-0 rounded-[var(--nova-radius)] border px-1.5 py-0.5 text-[10px] ${enabled ? 'border-[var(--nova-success)]/30 bg-[var(--nova-success-bg)] text-[var(--nova-success)]' : 'border-[var(--nova-danger)]/30 bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]'}`}>
            {enabled ? t('agents.option.on') : t('agents.option.off')}
          </span>
          {!availableForCurrent && (
            <span className="shrink-0 rounded bg-[var(--nova-danger-bg)] px-1.5 py-0.5 text-[10px] text-[var(--nova-danger)]">{t('agents.subAgents.unavailableShort')}</span>
          )}
        </div>
        <div className="mt-1 flex min-w-0 flex-wrap gap-x-2 gap-y-1 text-[11px] text-[var(--nova-text-faint)]">
          <span className="font-mono">{subAgent.id}</span>
          <span>{parents.map((parent) => t(`agents.subAgents.parent.${parent}`)).join(', ')}</span>
          <span>{subAgentToolSummary(t, subAgent.tools)}</span>
        </div>
      </div>
      <ToggleSwitch
        checked={enabled}
        onChange={onToggle}
        ariaLabel={t('agents.subAgents.enabled')}
      />
      <button type="button" onClick={onEdit} className="nova-icon-button flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('agents.subAgents.edit')}>
        <Edit3 className="h-3.5 w-3.5" />
      </button>
      <button type="button" onClick={onDelete} className="nova-icon-button flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-danger)]" aria-label={t('agents.subAgents.delete')}>
        <Trash2 className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

function SubAgentEditor({ id, agent, subAgent, toolRows, profiles, onChange }: {
  id: string
  agent: SubAgentParentKey
  subAgent: SubAgentConfig
  toolRows: AgentToolDefinition[]
  profiles: Array<{ id: string; label: string }>
  onChange: (id: string, patch: Partial<SubAgentConfig>) => void
}) {
  const { t } = useTranslation()
  const parents = effectiveSubAgentParents(subAgent)
  const parentSet = new Set(parents)
  const tools = subAgent.tools ?? {}
  const model = subAgent.model ?? {}
  const setModel = (patch: Partial<AgentModelOverride>) => onChange(id, { model: { ...model, ...patch } })
  const setTool = (key: ToolKey, value: boolean | null) => {
    const nextTools: AgentToolOverride = { ...tools }
    if (value === null) delete nextTools[key]
    else nextTools[key] = value
    onChange(id, { tools: nextTools })
  }
  const setParent = (parent: SubAgentParentKey, checked: boolean) => {
    const current = effectiveSubAgentParents(subAgent)
    const next = new Set(current)
    if (checked) next.add(parent)
    else next.delete(parent)
    const ordered = SUB_AGENT_PARENT_KEYS.filter((key) => next.has(key))
    onChange(id, { parents: ordered })
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="grid gap-3 md:grid-cols-2">
        <Field label={t('agents.subAgents.id')}>
          <Input aria-label={t('agents.subAgents.id')} value={subAgent.id ?? ''} onChange={(e) => onChange(id, { id: normalizeSubAgentID(e.target.value) })} className="h-7 text-xs" />
        </Field>
        <Field label={t('agents.subAgents.name')}>
          <Input aria-label={t('agents.subAgents.name')} value={subAgent.name ?? ''} onChange={(e) => onChange(id, { name: e.target.value })} className="h-7 text-xs" />
        </Field>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <Field label={t('agents.subAgents.description')}>
          <Input aria-label={t('agents.subAgents.description')} value={subAgent.description ?? ''} onChange={(e) => onChange(id, { description: e.target.value })} className="h-7 text-xs" />
        </Field>
        <Field label={t('agents.field.modelProfile')}>
          <Select value={model.profile_id || '__inherit__'} onValueChange={(profileID) => setModel({ profile_id: profileID === '__inherit__' ? '' : profileID })}>
            <SelectTrigger size="sm" className="w-full" aria-label={t('agents.field.modelProfile')}><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="__inherit__">{t('agents.option.inherit')}</SelectItem>
                {profiles.map((profile) => <SelectItem key={profile.id} value={profile.id}>{profile.label}</SelectItem>)}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      </div>
      <Field label={t('agents.subAgents.prompt')}>
        <Textarea
          autoResize
          aria-label={t('agents.subAgents.prompt')}
          value={subAgent.system_prompt ?? ''}
          onChange={(e) => onChange(id, { system_prompt: e.target.value })}
          placeholder={t('agents.subAgents.promptPlaceholder')}
          className="min-h-28 resize-y text-xs leading-5"
        />
      </Field>
      <Field label={t('agents.field.thinkingLevel')}>
        <Select value={model.thinking_level || '__inherit__'} onValueChange={(level) => setModel({ thinking_level: level === '__inherit__' ? '' : level })}>
          <SelectTrigger size="sm" className="w-full" aria-label={t('agents.field.thinkingLevel')}><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="__inherit__">{t('agents.option.inherit')}</SelectItem>
              {THINKING_LEVELS.map((level) => (
                <SelectItem key={level} value={level}>{t(`agents.thinkingLevel.${level}`)}</SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>
      <div>
        <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('agents.subAgents.parents')}</div>
        <div className="flex flex-wrap gap-2">
          {SUB_AGENT_PARENT_KEYS.map((parent) => (
            <label key={parent} className="inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
              <input type="checkbox" checked={parentSet.has(parent)} onChange={(e) => setParent(parent, e.target.checked)} />
              {t(`agents.subAgents.parent.${parent}`)}
            </label>
          ))}
        </div>
        {!parentSet.has(agent) && (
          <div className="mt-1.5 text-[11px] text-[var(--nova-danger)]">{t('agents.subAgents.notAvailableForCurrent')}</div>
        )}
      </div>
      <div>
        <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('agents.subAgents.tools')}</div>
        <div className="grid gap-2 md:grid-cols-2">
          {toolRows.map((tool) => {
            const explicit = tools[tool.key]
            const inherited = explicit === undefined || explicit === null
            const parentAllows = tool.allowed && tool.availability !== 'unavailable'
            const effective = parentAllows && (inherited || explicit)
            return (
              <div key={tool.key} className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5">
                <span className="min-w-0 flex-1 truncate text-[11px]">{t(tool.titleKey)}</span>
                {!parentAllows && (
                  <span className="shrink-0 rounded bg-[var(--nova-danger-bg)] px-1.5 py-0.5 text-[10px] text-[var(--nova-danger)]">
                    {t('agents.skills.unavailable')}
                  </span>
                )}
                {parentAllows ? (
                  <SwitchWithInheritance
                    checked={Boolean(effective)}
                    onChange={(checked) => setTool(tool.key, checked)}
                    ariaLabel={t(tool.titleKey)}
                    inherited={inherited}
                    onReset={!inherited ? () => setTool(tool.key, null) : undefined}
                  />
                ) : (
                  <span className="inline-flex shrink-0 items-center gap-1.5">
                    <Switch checked={false} disabled aria-label={t(tool.titleKey)} />
                    {inherited ? (
                      <span className="w-7 text-center text-[10px] leading-none text-[var(--nova-text-faint)]">{t('agents.badge.inherited')}</span>
                    ) : (
                      <button type="button" onClick={() => setTool(tool.key, null)} className="w-7 text-center text-[10px] leading-none text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]">
                        {t('agents.badge.overridden')}
                      </button>
                    )}
                  </span>
                )}
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

export function isSubAgentParent(agent: VisibleAgentKey): agent is SubAgentParentKey {
  return (SUB_AGENT_PARENT_KEYS as string[]).includes(agent)
}

const GENERAL_SUB_AGENT_KEYS = ['default', 'general', 'ide', 'interactive_story'] as const

function defaultGeneralSubAgentSettings(): Settings['general_sub_agents'] {
	return { default: false, general: true, ide: true }
}

export function previewGeneralSubAgentSettings(layered: LayeredSettings | null, activeLayer: SettingsLayer, draft: Settings): Settings['general_sub_agents'] {
  let settings = defaultGeneralSubAgentSettings()
  if (!layered) return mergeGeneralSubAgentSettings(settings, draft.general_sub_agents)
  settings = mergeGeneralSubAgentSettings(settings, layered.default.general_sub_agents)
  settings = mergeGeneralSubAgentSettings(settings, layered.global.general_sub_agents)
  settings = mergeGeneralSubAgentSettings(settings, activeLayer === 'user' ? draft.general_sub_agents : layered.user.general_sub_agents)
  settings = mergeGeneralSubAgentSettings(settings, activeLayer === 'workspace' ? draft.general_sub_agents : layered.workspace.general_sub_agents)
  return settings
}

function mergeGeneralSubAgentSettings(parent: Settings['general_sub_agents'], child: Settings['general_sub_agents']): Settings['general_sub_agents'] {
  const out: Settings['general_sub_agents'] = { ...(parent ?? {}) }
  if (!child) return out
  for (const key of GENERAL_SUB_AGENT_KEYS) {
    const value = child[key]
    if (value !== undefined && value !== null) out[key] = value
  }
  return out
}

function resolveGeneralSubAgentEnabled(settings: Settings['general_sub_agents'], agent: SubAgentParentKey) {
  const fallback = settings?.default ?? false
  return settings?.[agent] ?? fallback
}

function subAgentToolSummary(t: (key: string, options?: Record<string, unknown>) => string, tools?: AgentToolOverride) {
  const restrictionCount = Object.values(tools ?? {}).filter((value) => value === false).length
  if (restrictionCount === 0) return t('agents.subAgents.toolsInherited')
  return t('agents.subAgents.toolsRestricted', { count: restrictionCount })
}

function nextSubAgentID(current: SubAgentConfig[]) {
  const used = new Set(current.map((subAgent) => subAgent.id).filter(Boolean))
  for (let index = 1; index < 1000; index += 1) {
    const id = `subagent-${index}`
    if (!used.has(id)) return id
  }
  return `subagent-${Date.now()}`
}

function normalizeSubAgentID(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/[-_]{2,}/g, '-')
    .replace(/^[-_]+|[-_]+$/g, '')
}

function normalizeSubAgentConfig(value: SubAgentConfig): SubAgentConfig {
  return {
    ...value,
    id: normalizeSubAgentID(value.id || ''),
    name: value.name ?? '',
    description: value.description ?? '',
    system_prompt: value.system_prompt ?? '',
    parents: sanitizeSubAgentParents(value.parents),
  }
}

function mergeVisibleSubAgents(effective: SubAgentConfig[], draft: SubAgentConfig[]) {
  const rows: SubAgentConfig[] = []
  const index = new Map<string, number>()
  for (const subAgent of effective) {
    const id = normalizeSubAgentID(subAgent.id || '')
    if (!id || index.has(id)) continue
    index.set(id, rows.length)
    rows.push(normalizeSubAgentConfig(subAgent))
  }
  for (const subAgent of draft) {
    const normalized = normalizeSubAgentConfig(subAgent)
    const id = normalizeSubAgentID(normalized.id || '')
    if (!id) continue
    const existing = index.get(id)
    if (existing === undefined) {
      index.set(id, rows.length)
      rows.push(normalized)
    } else {
      rows[existing] = normalized
    }
  }
  return rows
}

function upsertSubAgentOverride(current: SubAgentConfig[], next: SubAgentConfig, previousID?: string) {
  const nextID = normalizeSubAgentID(next.id || '')
  const oldID = normalizeSubAgentID(previousID || nextID)
  if (!nextID) return current
  const filtered = current.filter((subAgent) => {
    const id = normalizeSubAgentID(subAgent.id || '')
    return id !== nextID && id !== oldID
  })
  return [...filtered, next]
}

function sanitizeSubAgentParents(value?: string[]) {
  if (!value || value.length === 0) return []
  const selected = SUB_AGENT_PARENT_KEYS.filter((parent) => value.includes(parent))
  return selected
}

function effectiveSubAgentParents(subAgent: SubAgentConfig): SubAgentParentKey[] {
  const parents = sanitizeSubAgentParents(subAgent.parents)
  return parents as SubAgentParentKey[]
}

function subAgentParentsWithout(subAgent: SubAgentConfig, agent: SubAgentParentKey): SubAgentParentKey[] {
  return effectiveSubAgentParents(subAgent).filter((parent) => parent !== agent)
}
