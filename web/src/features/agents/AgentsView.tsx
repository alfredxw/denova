import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ElementType, ReactNode } from 'react'
import { Bot, Brain, Check, Database, FileText, FolderOpen, ListChecks, MessageSquareText, PenLine, Save, Search, Settings2, Shield, Sparkles, Terminal, Wrench, X } from 'lucide-react'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { fetchSettings, updateUserSettings, updateWorkspaceSettings } from '@/features/settings/api'
import type { AgentModelOverride, AgentModelSettings, AgentToolOverride, LayeredSettings, ModelProfileSettings, Settings, SettingsLayer } from '@/features/settings/types'

type AgentKey = keyof AgentModelSettings
type VisibleAgentKey = Exclude<AgentKey, 'default'>
type ToolKey = keyof AgentToolOverride

const fieldCls = 'nova-field min-h-7 flex-1 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[#3a3a3a] focus:bg-[var(--nova-surface-3)]'
const tabCls = 'nova-nav-item rounded-[var(--nova-radius)] px-2.5 py-1 text-xs'

const AGENTS: Array<{
  key: VisibleAgentKey
  title: string
  subtitle: string
  group: string
  capabilityMode: 'tools' | 'built_in' | 'model_only'
  icon: ElementType
}> = [
  { key: 'ide', title: 'IDE 创作 Agent', subtitle: '章节续写、文件编辑、设定同步', group: '创作', capabilityMode: 'tools', icon: PenLine },
  { key: 'lore_editor', title: '资料库 Agent', subtitle: '资料条目的结构化整理', group: '创作', capabilityMode: 'built_in', icon: Database },
  { key: 'teller_editor', title: '讲述者 Agent', subtitle: '讲述者规则创建与修改', group: '创作', capabilityMode: 'built_in', icon: Settings2 },
  { key: 'interactive_story', title: '互动叙事 Agent', subtitle: '故事舞台回合生成', group: '互动', capabilityMode: 'tools', icon: MessageSquareText },
  { key: 'interactive_state', title: '状态记忆 Agent', subtitle: '互动回合状态提取', group: '互动', capabilityMode: 'model_only', icon: Shield },
  { key: 'interactive_hot_choices', title: '快捷选项 Agent', subtitle: '生成可选行动', group: '互动', capabilityMode: 'model_only', icon: Sparkles },
  { key: 'version_summary', title: '版本说明 Agent', subtitle: '自动版本摘要', group: '版本', capabilityMode: 'model_only', icon: ListChecks },
]

const TOOL_ROWS: Array<{ key: ToolKey; title: string; subtitle: string; icon: ElementType }> = [
  { key: 'file_read', title: '读取与搜索文件', subtitle: 'ls / read_file / glob / grep', icon: Search },
  { key: 'file_write', title: '修改文件', subtitle: 'write_file / edit_file', icon: FileText },
  { key: 'shell_execute', title: '命令执行', subtitle: 'execute', icon: Terminal },
  { key: 'skills', title: 'Skills', subtitle: '从 Skills 目录加载创作技能', icon: FolderOpen },
  { key: 'lore_read', title: '读取资料库', subtitle: 'read_lore_items / search_lore_items', icon: Database },
  { key: 'lore_write', title: '写入资料库', subtitle: 'write_lore_items', icon: Wrench },
  { key: 'todo', title: '任务清单', subtitle: 'write_todos', icon: ListChecks },
]

const BASE_TOOL_VALUES: Required<AgentToolOverride> = { file_read: true, file_write: true, shell_execute: true, skills: true, lore_read: true, lore_write: true, todo: true }

const FALLBACK_AGENT_TOOL_VALUES: Record<VisibleAgentKey, Required<AgentToolOverride>> = {
  ide: { file_read: true, file_write: true, shell_execute: true, skills: true, lore_read: true, lore_write: true, todo: true },
  interactive_story: { file_read: true, file_write: true, shell_execute: true, skills: false, lore_read: true, lore_write: false, todo: false },
  lore_editor: disabledTools(),
  teller_editor: disabledTools(),
  interactive_state: disabledTools(),
  interactive_hot_choices: disabledTools(),
  version_summary: disabledTools(),
}

export function AgentsView({ onClose }: { onClose?: () => void }) {
  const [layered, setLayered] = useState<LayeredSettings | null>(null)
  const [activeLayer, setActiveLayer] = useState<SettingsLayer>('workspace')
  const [activeAgent, setActiveAgent] = useState<VisibleAgentKey>('ide')
  const [draft, setDraft] = useState<Settings>({})
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const data = await fetchSettings()
      setLayered(data)
      setDraft(activeLayer === 'user' ? data.user : data.workspace)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [activeLayer])

  useEffect(() => { void load() }, [load])

  useEffect(() => {
    if (!layered) return
    setDraft(activeLayer === 'user' ? layered.user : layered.workspace)
  }, [activeLayer, layered])

  const effective = layered?.effective ?? {}
  const selected = AGENTS.find((agent) => agent.key === activeAgent) ?? AGENTS[0]
  const profileOptions = useMemo(() => buildProfileOptions(draft, effective), [draft, effective])
  const modelValue = draft.agent_models?.[activeAgent] ?? {}
  const inheritedModel = mergeAgentModelOverride(effective.agent_models?.default ?? {}, effective.agent_models?.[activeAgent] ?? {})
  const toolValue = draft.agent_tools?.[activeAgent] ?? {}
  const inheritedTools = effective.agent_tools?.[activeAgent] ?? FALLBACK_AGENT_TOOL_VALUES[activeAgent]
  const effectiveTools = resolveEffectiveTools(effective.agent_tools?.default ?? {}, inheritedTools)

  const onSave = async () => {
    setSaving(true)
    setError(null)
    try {
      const updater = activeLayer === 'user' ? updateUserSettings : updateWorkspaceSettings
      const next = await updater(draft)
      setLayered(next)
      window.dispatchEvent(new CustomEvent('nova:settings-updated'))
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const setAgentModel = (patch: Partial<AgentModelOverride>) => {
    setDraft((current) => ({
      ...current,
      agent_models: {
        ...(current.agent_models ?? {}),
        [activeAgent]: { ...(current.agent_models?.[activeAgent] ?? {}), ...patch },
      },
    }))
  }

  const setAgentTool = (key: ToolKey, value: boolean | null) => {
    setDraft((current) => ({
      ...current,
      agent_tools: {
        ...(current.agent_tools ?? {}),
        [activeAgent]: { ...(current.agent_tools?.[activeAgent] ?? {}), [key]: value },
      },
    }))
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="nova-topbar flex min-h-10 shrink-0 flex-wrap items-center gap-2 border-b px-4 py-1.5 text-xs">
        <Bot className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <span className="font-medium">Agents</span>
        <div className="ml-3 flex gap-1 border-l border-[var(--nova-border)] pl-3">
          {(['workspace', 'user'] as SettingsLayer[]).map((layer) => (
            <button
              key={layer}
              type="button"
              onClick={() => setActiveLayer(layer)}
              className={`${tabCls} ${activeLayer === layer ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
            >
              {layer === 'workspace' ? '当前工作区' : '用户配置'}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={onSave}
          disabled={saving}
          className="nova-nav-item ml-auto inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 py-1 text-[var(--nova-text)] disabled:opacity-50"
        >
          <Save className="h-3.5 w-3.5" />
          {saving ? '保存中…' : '保存'}
        </button>
        {onClose && (
          <button type="button" onClick={onClose} className="nova-nav-item rounded p-1" aria-label="关闭 Agents" title="关闭 Agents">
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {error && <InlineErrorNotice className="mx-3 mt-2" message={error} title="配置保存失败" />}

      <div className="grid min-h-0 flex-1 grid-cols-[18rem_minmax(0,1fr)] text-xs">
        <aside className="min-h-0 overflow-y-auto border-r border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <AgentList active={activeAgent} onSelect={setActiveAgent} />
        </aside>

        <main className="min-h-0 overflow-y-auto">
          <div className="mx-auto flex max-w-5xl flex-col gap-5 px-6 py-5">
            <AgentHeader agent={selected} />
            <AgentModelSection
              value={modelValue}
              inherited={inheritedModel}
              profiles={profileOptions}
              onChange={setAgentModel}
            />
            {selected.capabilityMode === 'tools' ? (
              <AgentToolSection
                value={toolValue}
                effective={effectiveTools}
                onChange={setAgentTool}
              />
            ) : selected.capabilityMode === 'built_in' ? (
              <AgentBuiltInCapabilitySection agent={selected.key} />
            ) : (
              <AgentModelOnlySection />
            )}
            <AgentContextSection agent={selected.key} effective={effective} />
          </div>
        </main>
      </div>
    </div>
  )
}

function AgentList({ active, onSelect }: { active: VisibleAgentKey; onSelect: (agent: VisibleAgentKey) => void }) {
  const groups = AGENTS.reduce<Array<{ group: string; agents: typeof AGENTS }>>((acc, agent) => {
    const last = acc[acc.length - 1]
    if (last?.group === agent.group) last.agents.push(agent)
    else acc.push({ group: agent.group, agents: [agent] })
    return acc
  }, [])

  return (
    <nav className="space-y-4">
      {groups.map((group) => (
        <div key={group.group}>
          <div className="mb-1.5 px-2 text-[11px] font-medium text-[var(--nova-text-faint)]">{group.group}</div>
          <div className="space-y-1">
            {group.agents.map((agent) => {
              const Icon = agent.icon
              return (
                <button
                  key={agent.key}
                  type="button"
                  onClick={() => onSelect(agent.key)}
                  className={`nova-nav-item flex w-full items-center gap-2 rounded-[var(--nova-radius)] px-2.5 py-2 text-left ${active === agent.key ? 'is-active' : ''}`}
                >
                  <Icon className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
                  <span className="min-w-0">
                    <span className="block truncate font-medium text-[var(--nova-text)]">{agent.title}</span>
                    <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{agent.subtitle}</span>
                  </span>
                </button>
              )
            })}
          </div>
        </div>
      ))}
    </nav>
  )
}

function AgentHeader({ agent }: { agent: (typeof AGENTS)[number] }) {
  const Icon = agent.icon
  return (
    <section className="border-b border-[var(--nova-border)] pb-4">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <Icon className="h-4 w-4 text-[var(--nova-text-muted)]" />
        </div>
        <div className="min-w-0">
          <h1 className="truncate text-sm font-semibold">{agent.title}</h1>
          <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{agent.subtitle}</div>
        </div>
      </div>
    </section>
  )
}

function AgentModelSection({ value, inherited, profiles, onChange }: {
  value: AgentModelOverride
  inherited: AgentModelOverride
  profiles: Array<{ id: string; label: string }>
  onChange: (patch: Partial<AgentModelOverride>) => void
}) {
  const hasProfile = hasTextOverride(value.profile_id)
  const hasTemperature = value.temperature !== undefined && value.temperature !== null
  const hasThinking = value.enable_thinking !== undefined && value.enable_thinking !== null
  const hasEffort = hasTextOverride(value.reasoning_effort)
  const effectiveProfile = hasProfile ? value.profile_id || 'default' : inherited.profile_id || 'default'
  const effectiveTemperature = hasTemperature ? value.temperature : inherited.temperature
  const effectiveThinking = hasThinking ? value.enable_thinking : inherited.enable_thinking
  const effectiveEffort = hasEffort ? value.reasoning_effort || '' : inherited.reasoning_effort || ''

  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Brain} title="模型与思考" />
      <div className="grid gap-3 md:grid-cols-2">
        <Field label="模型配置" inherited={!hasProfile} onReset={hasProfile ? () => onChange({ profile_id: '' }) : undefined}>
          <select value={effectiveProfile} onChange={(e) => onChange({ profile_id: e.target.value })} className={fieldCls}>
            {profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.label}</option>)}
          </select>
        </Field>
        <Field label="Temperature" inherited={!hasTemperature} onReset={hasTemperature ? () => onChange({ temperature: null }) : undefined}>
          <input
            type="number"
            step={0.1}
            min={0}
            max={2}
            value={effectiveTemperature ?? ''}
            placeholder="平台默认"
            onChange={(e) => onChange({ temperature: e.target.value === '' ? null : Number(e.target.value) })}
            className={fieldCls}
          />
        </Field>
        <Field label="思考开关" inherited={!hasThinking} onReset={hasThinking ? () => onChange({ enable_thinking: null }) : undefined}>
          <select
            value={effectiveThinking === null || effectiveThinking === undefined ? '' : String(effectiveThinking)}
            onChange={(e) => onChange({ enable_thinking: e.target.value === '' ? null : e.target.value === 'true' })}
            className={fieldCls}
          >
            <option value="">不传</option>
            <option value="true">开启</option>
            <option value="false">关闭</option>
          </select>
        </Field>
        <Field label="推理强度" inherited={!hasEffort} onReset={hasEffort ? () => onChange({ reasoning_effort: '' }) : undefined}>
          <select value={effectiveEffort} onChange={(e) => onChange({ reasoning_effort: e.target.value })} className={fieldCls}>
            <option value="">不传</option>
            <option value="low">low</option>
            <option value="medium">medium</option>
            <option value="high">high</option>
          </select>
        </Field>
      </div>
    </section>
  )
}

function AgentToolSection({ value, effective, onChange }: {
  value: AgentToolOverride
  effective: Required<AgentToolOverride>
  onChange: (key: ToolKey, value: boolean | null) => void
}) {
  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Wrench} title="工具能力" />
      <div className="grid gap-2 lg:grid-cols-2">
        {TOOL_ROWS.map((tool) => {
          const Icon = tool.icon
          const explicit = value[tool.key]
          const inherited = explicit === undefined || explicit === null
          const current = inherited ? effective[tool.key] : explicit
          return (
            <div key={tool.key} className="flex min-h-16 items-center gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
              <Icon className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
              <div className="min-w-0 flex-1">
                <div className="truncate font-medium">{tool.title}</div>
                <div className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">{tool.subtitle}</div>
              </div>
              <select
                value={String(current)}
                onChange={(e) => onChange(tool.key, e.target.value === '' ? null : e.target.value === 'true')}
                className={`${fieldCls} max-w-32 shrink-0`}
              >
                <option value="true">开启</option>
                <option value="false">关闭</option>
              </select>
              <InheritanceBadge inherited={inherited} onReset={!inherited ? () => onChange(tool.key, null) : undefined} />
            </div>
          )
        })}
      </div>
    </section>
  )
}

function AgentBuiltInCapabilitySection({ agent }: { agent: VisibleAgentKey }) {
  const rows = builtInCapabilityRows(agent)
  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Wrench} title="内置能力" />
      <div className="grid gap-2 md:grid-cols-2">
        {rows.map((row) => (
          <div key={row.title} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
            <div className="font-medium text-[var(--nova-text)]">{row.title}</div>
            <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{row.value}</div>
          </div>
        ))}
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        这些写入能力由应用层执行：模型先生成结构化编辑方案，后端校验后保存；不是 deep-agent 文件/命令/Skills 工具链，所以这里不提供单项工具开关。
      </div>
    </section>
  )
}

function AgentModelOnlySection() {
  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Wrench} title="工具能力" />
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        这个 Agent 当前是纯模型调用，不修改文件、资料库或讲述者；这里只配置模型与思考参数。
      </div>
    </section>
  )
}

function AgentContextSection({ agent, effective }: { agent: VisibleAgentKey; effective: Settings }) {
  const rows = contextRowsFor(agent, effective)
  return (
    <section className="space-y-3 pb-5">
      <SectionTitle icon={FolderOpen} title="上下文" />
      <div className="grid gap-2 md:grid-cols-3">
        {rows.map((row) => (
          <div key={row.title} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
            <div className="flex items-center gap-1.5 text-[var(--nova-text)]">
              <Check className="h-3.5 w-3.5 text-[var(--nova-accent-green)]" />
              <span className="font-medium">{row.title}</span>
            </div>
            <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">{row.value}</div>
          </div>
        ))}
      </div>
    </section>
  )
}

function SectionTitle({ icon: Icon, title }: { icon: ElementType; title: string }) {
  return (
    <div className="flex items-center gap-2 text-xs font-medium">
      <Icon className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
      {title}
    </div>
  )
}

function Field({ label, inherited, onReset, children }: { label: string; inherited?: boolean; onReset?: () => void; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[var(--nova-text-muted)]">{label}</span>
      <span className="flex items-center gap-2">
        {children}
        {inherited !== undefined && <InheritanceBadge inherited={inherited} onReset={onReset} />}
      </span>
    </div>
  )
}

function InheritanceBadge({ inherited, onReset }: { inherited: boolean; onReset?: () => void }) {
  return (
    <span className={`inline-flex h-7 shrink-0 items-center rounded-[var(--nova-radius)] border px-2 text-[11px] ${inherited ? 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-faint)]' : 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text-muted)]'}`}>
      {inherited ? '继承' : (
        <button type="button" onClick={onReset} className="text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]">
          已覆盖
        </button>
      )}
    </span>
  )
}

function buildProfileOptions(draft: Settings, effective: Settings): Array<{ id: string; label: string }> {
  const profiles = new Map<string, string>()
  const add = (profile?: ModelProfileSettings) => {
    const id = profile?.id?.trim()
    if (!id) return
    profiles.set(id, profile?.name || profile?.openai_model || id)
  }
  profiles.set('default', effective.openai_model || '默认模型')
  ;(effective.model_profiles ?? []).forEach(add)
  ;(draft.model_profiles ?? []).forEach(add)
  return Array.from(profiles.entries()).map(([id, label]) => ({ id, label: id === 'default' ? `default（${label}）` : `${id}（${label}）` }))
}

function contextRowsFor(agent: VisibleAgentKey, effective: Settings) {
  if (agent === 'ide') {
    return [
      { title: '当前书籍', value: 'workspace' },
      { title: '默认讲述者', value: effective.ide_story_teller_id || 'classic' },
      { title: '会话上下文', value: '当前会话有效历史' },
    ]
  }
  if (agent === 'interactive_story') {
    return [
      { title: '故事状态', value: 'story jsonl' },
      { title: '讲述者', value: '当前故事讲述者' },
      { title: '资料索引', value: '常驻与自动匹配资料' },
    ]
  }
  return [
    { title: '输入来源', value: '当前操作请求' },
    { title: '输出形态', value: '结构化 JSON 或摘要' },
    { title: '历史边界', value: '不读取创作对话上下文' },
  ]
}

function builtInCapabilityRows(agent: VisibleAgentKey) {
  if (agent === 'lore_editor') {
    return [
      { title: '读取资料库', value: '读取当前资料条目和用户引用的资料上下文。' },
      { title: '写入资料库', value: '生成 create / update / delete 操作，由后端校验后应用到资料库。' },
    ]
  }
  if (agent === 'teller_editor') {
    return [
      { title: '读取讲述者', value: '读取当前讲述者列表、选中讲述者和用户引用的讲述者。' },
      { title: '写入讲述者', value: '生成 create / update 方案，由后端校验后保存为讲述者规则包。' },
    ]
  }
  return []
}

function disabledTools(): Required<AgentToolOverride> {
  return { file_read: false, file_write: false, shell_execute: false, skills: false, lore_read: false, lore_write: false, todo: false }
}

function resolveEffectiveTools(defaultTools: AgentToolOverride, tools: AgentToolOverride): Required<AgentToolOverride> {
  return {
    file_read: tools.file_read ?? defaultTools.file_read ?? BASE_TOOL_VALUES.file_read,
    file_write: tools.file_write ?? defaultTools.file_write ?? BASE_TOOL_VALUES.file_write,
    shell_execute: tools.shell_execute ?? defaultTools.shell_execute ?? BASE_TOOL_VALUES.shell_execute,
    skills: tools.skills ?? defaultTools.skills ?? BASE_TOOL_VALUES.skills,
    lore_read: tools.lore_read ?? defaultTools.lore_read ?? BASE_TOOL_VALUES.lore_read,
    lore_write: tools.lore_write ?? defaultTools.lore_write ?? BASE_TOOL_VALUES.lore_write,
    todo: tools.todo ?? defaultTools.todo ?? BASE_TOOL_VALUES.todo,
  }
}

function hasTextOverride(value?: string) {
  return value !== undefined && value !== ''
}

function mergeAgentModelOverride(parent: AgentModelOverride, child: AgentModelOverride): AgentModelOverride {
  return {
    profile_id: child.profile_id || parent.profile_id,
    temperature: child.temperature ?? parent.temperature,
    enable_thinking: child.enable_thinking ?? parent.enable_thinking,
    reasoning_effort: child.reasoning_effort || parent.reasoning_effort,
  }
}
