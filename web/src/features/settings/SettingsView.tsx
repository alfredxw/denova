import { useEffect, useRef, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, ChevronUp, Plus, Save, Settings as SettingsIcon, Trash2, X } from 'lucide-react'
import type { LayeredSettings, ModelProfileSettings, Settings, SettingsLayer } from './types'
import { fetchSettings, updateUserSettings, updateWorkspaceSettings } from './api'
import { FONT_OPTIONS, fontLabelFor } from './font-options'
import { getInteractiveTellers } from '@/features/interactive/api'
import type { Teller } from '@/features/interactive/types'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'

type SettingsSectionId = 'model' | 'paths' | 'appearance' | 'agent' | 'ide-editor' | 'versions' | 'interactive'

type SettingsSection = {
  id: SettingsSectionId
  group: '公共配置' | 'IDE 模式' | '互动模式'
  title: string
  children: ReactNode
}

const tabCls = 'nova-nav-item rounded-[var(--nova-radius)] px-2.5 py-1 text-xs'
const fieldCls = 'nova-field min-h-7 flex-1 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[#3a3a3a] focus:bg-[var(--nova-surface-3)]'
const iconButtonCls = 'nova-nav-item rounded-[var(--nova-radius)] text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

export function SettingsView({ onClose }: { onClose?: () => void }) {
  const [layered, setLayered] = useState<LayeredSettings | null>(null)
  const [activeLayer, setActiveLayer] = useState<SettingsLayer>('user')
  const [draft, setDraft] = useState<Settings>({})
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [availableTellers, setAvailableTellers] = useState<Teller[]>([])
  const [activeSection, setActiveSection] = useState<SettingsSectionId>('model')
  const [expandedSections, setExpandedSections] = useState<Record<SettingsSectionId, boolean>>({
    model: true,
    paths: true,
    appearance: true,
    agent: true,
    'ide-editor': true,
    versions: true,
    interactive: true,
  })
  const contentRef = useRef<HTMLDivElement | null>(null)
  const sectionRefs = useRef<Record<string, HTMLElement | null>>({})

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
    if (activeLayer !== 'workspace') return
    getInteractiveTellers()
      .then((items) => setAvailableTellers(items))
      .catch((e) => console.warn('[settings] 获取讲述者列表失败', e))
  }, [activeLayer])

  useEffect(() => {
    if (!layered) return
    setDraft(activeLayer === 'user' ? layered.user : layered.workspace)
  }, [activeLayer, layered])

  const effective = layered?.effective ?? {}
  const onSave = async () => {
    setSaving(true)
    setError(null)
    try {
      const updater = activeLayer === 'user' ? updateUserSettings : updateWorkspaceSettings
      const next = await updater(draft)
      setLayered(next)
      // 通知应用层重新读取分层配置（如 max_open_tabs 等需要立即生效的设置）
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('nova:settings-updated'))
      }
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const setField = <K extends keyof Settings>(k: K, v: Settings[K]) =>
    setDraft((d) => ({ ...d, [k]: v }))

  const setModelProfiles = (profiles: ModelProfileSettings[]) => {
    setField('model_profiles', profiles)
  }

  const placeholderFor = (k: keyof Settings): string => {
    const v = effective[k]
    if (v === undefined || v === null || v === '') return '未设置'
    return `继承：${String(v)}`
  }

  const sections: SettingsSection[] = [
    {
      id: 'model',
      group: '公共配置',
      title: '模型',
      children: (
        <>
          <Text label="API Key" value={draft.openai_api_key} placeholder={placeholderFor('openai_api_key')}
                onChange={(v) => setField('openai_api_key', v)} type="password" />
          <Text label="Base URL" value={draft.openai_base_url} placeholder={placeholderFor('openai_base_url')}
                onChange={(v) => setField('openai_base_url', v)} />
          <Text label="模型" value={draft.openai_model} placeholder={placeholderFor('openai_model')}
                onChange={(v) => setField('openai_model', v)} />
          <ModelProfilesEditor
            profiles={draft.model_profiles ?? []}
            effectiveProfiles={effective.model_profiles ?? []}
            onChange={setModelProfiles}
          />
        </>
      ),
    },
    {
      id: 'paths',
      group: '公共配置',
      title: '路径',
      children: (
        <>
          <Text label="Skills 目录" value={draft.skills_dir} placeholder={placeholderFor('skills_dir')}
                onChange={(v) => setField('skills_dir', v)} />
          <ReadOnly label="Nova 数据目录" value={layered?.paths?.nova_dir} />
          <ReadOnly label="用户配置文件" value={layered?.paths?.user_config} />
          <ReadOnly label="工作区配置文件" value={layered?.paths?.workspace_config} />
        </>
      ),
    },
    {
      id: 'appearance',
      group: '公共配置',
      title: '外观',
      children: (
        <>
          <FontSelect label="界面字体" value={draft.ui_font_family}
                      effective={effective.ui_font_family}
                      onChange={(v) => setField('ui_font_family', v)} />
          <Num label="界面字号 (px)" value={draft.ui_font_size ?? null}
               placeholder={placeholderFor('ui_font_size')}
               min={11}
               max={16}
               onChange={(v) => setField('ui_font_size', v)} />
          <FontSelect label="阅读字体" value={draft.reading_font_family}
                      effective={effective.reading_font_family}
                      onChange={(v) => setField('reading_font_family', v)} />
          <Num label="阅读字号 (px)" value={draft.reading_font_size ?? null}
               placeholder={placeholderFor('reading_font_size')}
               min={14}
               max={28}
               onChange={(v) => setField('reading_font_size', v)} />
        </>
      ),
    },
    {
      id: 'agent',
      group: '公共配置',
      title: 'Agent',
      children: (
        <>
          <Num label="最大迭代轮数" value={draft.max_iteration ?? null}
               placeholder={placeholderFor('max_iteration')}
               onChange={(v) => setField('max_iteration', v)} />
          <Num label="模型重试次数" value={draft.model_max_retries ?? null}
               placeholder={placeholderFor('model_max_retries')}
               onChange={(v) => setField('model_max_retries', v)} />
          <BoolTri label="默认 PlanMode" value={draft.plan_mode_default ?? null}
                   effective={effective.plan_mode_default}
                   onChange={(v) => setField('plan_mode_default', v)} />
        </>
      ),
    },
    {
      id: 'ide-editor',
      group: 'IDE 模式',
      title: '编辑器',
      children: (
        <>
          <BoolTri label="自动保存" value={draft.auto_save_enabled ?? null}
                   effective={effective.auto_save_enabled}
                   onChange={(v) => setField('auto_save_enabled', v)} />
          <Num label="自动保存间隔 (ms)" value={draft.auto_save_interval_ms ?? null}
               placeholder={placeholderFor('auto_save_interval_ms')}
               onChange={(v) => setField('auto_save_interval_ms', v)} />
          <Text label="章节文件名模板" value={draft.chapter_filename_format}
                placeholder={placeholderFor('chapter_filename_format')}
                onChange={(v) => setField('chapter_filename_format', v)} />
          <Num label="最大同时打开 Tab 数" value={draft.max_open_tabs ?? null}
               placeholder={placeholderFor('max_open_tabs')}
               onChange={(v) => setField('max_open_tabs', v)} />
          <BoolTri label="启用草稿流程" value={draft.draft_flow_enabled ?? null}
                   effective={effective.draft_flow_enabled}
                   onChange={(v) => setField('draft_flow_enabled', v)} />
          <Num label="章节组最少章节" value={draft.chapter_group_min ?? null}
               placeholder={placeholderFor('chapter_group_min')}
               onChange={(v) => setField('chapter_group_min', v)} />
          <Num label="章节组最多章节" value={draft.chapter_group_max ?? null}
               placeholder={placeholderFor('chapter_group_max')}
               onChange={(v) => setField('chapter_group_max', v)} />
          {activeLayer === 'workspace' && (
            <TellerSelect
              label="默认讲述者"
              value={draft.ide_story_teller_id}
              effective={effective.ide_story_teller_id}
              tellers={availableTellers}
              onChange={(v) => setField('ide_story_teller_id', v)}
            />
          )}
        </>
      ),
    },
    {
      id: 'versions',
      group: 'IDE 模式',
      title: '版本管理',
      children: activeLayer === 'workspace' ? (
        <>
          <BoolTri label="定时自动保存版本" value={draft.version_timed_enabled ?? null}
                   effective={effective.version_timed_enabled}
                   onChange={(v) => setField('version_timed_enabled', v)} />
          <Num label="定时保存间隔 (分钟)" value={draft.version_timed_interval_minutes ?? null}
               placeholder={placeholderFor('version_timed_interval_minutes')}
               onChange={(v) => setField('version_timed_interval_minutes', v)} />
          <BoolTri label="Agent 大量输出自动保存" value={draft.version_agent_enabled ?? null}
                   effective={effective.version_agent_enabled}
                   onChange={(v) => setField('version_agent_enabled', v)} />
          <Num label="Agent 触发字数" value={draft.version_agent_char_threshold ?? null}
               placeholder={placeholderFor('version_agent_char_threshold')}
               onChange={(v) => setField('version_agent_char_threshold', v)} />
          <Num label="自动版本保留数量" value={draft.version_auto_retention ?? null}
               placeholder={placeholderFor('version_auto_retention')}
               onChange={(v) => setField('version_auto_retention', v)} />
        </>
      ) : (
        <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">版本管理策略按每本书单独保存，请切换到工作区配置后修改。</div>
      ),
    },
    {
      id: 'interactive',
      group: '互动模式',
      title: '故事舞台',
      children: activeLayer === 'workspace' ? (
        <>
          <Num label="最大输出 Token" value={draft.interactive_max_tokens ?? null}
               placeholder="不填则不限制，优先避免截断"
               onChange={(v) => setField('interactive_max_tokens', v)} />
          <BoolTri label="输入框快捷选择" value={draft.interactive_hot_choices_enabled ?? null}
                   effective={effective.interactive_hot_choices_enabled}
                   onChange={(v) => setField('interactive_hot_choices_enabled', v)} />
          <Num label="故事舞台行间距" value={draft.interactive_stage_line_height ?? null}
               placeholder={placeholderFor('interactive_stage_line_height')}
               step={0.05}
               onChange={(v) => setField('interactive_stage_line_height', v)} />
        </>
      ) : (
        <>
          <BoolTri label="输入框快捷选择" value={draft.interactive_hot_choices_enabled ?? null}
                   effective={effective.interactive_hot_choices_enabled}
                   onChange={(v) => setField('interactive_hot_choices_enabled', v)} />
          <Num label="故事舞台行间距" value={draft.interactive_stage_line_height ?? null}
               placeholder={placeholderFor('interactive_stage_line_height')}
               step={0.05}
               onChange={(v) => setField('interactive_stage_line_height', v)} />
        </>
      ),
    },
  ]

  const jumpToSection = (id: SettingsSectionId) => {
    setActiveSection(id)
    setExpandedSections((prev) => ({ ...prev, [id]: true }))
    requestAnimationFrame(() => {
      sectionRefs.current[id]?.scrollIntoView({ block: 'start', behavior: 'smooth' })
    })
  }

  const toggleSection = (id: SettingsSectionId) => {
    setExpandedSections((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const onContentScroll = () => {
    const container = contentRef.current
    if (!container) return
    const top = container.getBoundingClientRect().top
    const current = sections.reduce<SettingsSectionId>((acc, section) => {
      const node = sectionRefs.current[section.id]
      if (!node) return acc
      return node.getBoundingClientRect().top <= top + 72 ? section.id : acc
    }, sections[0]?.id ?? 'model')
    if (current !== activeSection) setActiveSection(current)
  }

  const navGroups = sections.reduce<Array<{ group: SettingsSection['group']; items: SettingsSection[] }>>((groups, section) => {
    const last = groups[groups.length - 1]
    if (last?.group === section.group) {
      last.items.push(section)
    } else {
      groups.push({ group: section.group, items: [section] })
    }
    return groups
  }, [])

  return (
    <div className="nova-settings-view flex h-full min-h-0 w-full flex-col text-[var(--nova-text)]">
      <div className="nova-topbar flex min-h-10 shrink-0 flex-wrap items-center gap-2 border-b px-4 py-1.5 text-xs">
        <SettingsIcon className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <span className="font-medium text-[var(--nova-text)]">设置</span>
        <div className="ml-3 flex gap-1 border-l border-[var(--nova-border)] pl-3">
          {(['user', 'workspace'] as SettingsLayer[]).map((l) => (
            <button
              key={l}
              type="button"
              onClick={() => setActiveLayer(l)}
              className={`${tabCls} ${
                activeLayer === l ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'
              }`}
            >
              {l === 'user' ? '用户配置' : '当前工作区'}
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
          <button
            type="button"
            onClick={onClose}
            className={`${iconButtonCls} p-1`}
            aria-label="关闭设置"
            title="关闭设置"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {error && <InlineErrorNotice className="mx-3 mt-2" message={error} title="配置保存失败" />}

      <div className="flex min-h-0 flex-1 text-xs">
        <aside className="w-44 shrink-0 border-r border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-4 sm:w-52 sm:px-3 md:w-56">
          <nav className="space-y-4">
            {navGroups.map((group) => (
              <div key={group.group}>
                <div className="mb-1.5 px-2 text-[11px] font-medium text-[var(--nova-text-faint)]">{group.group}</div>
                <div className="space-y-1">
                  {group.items.map((section) => (
                    <button
                      key={section.id}
                      type="button"
                      onClick={() => jumpToSection(section.id)}
                      className={`nova-nav-item flex w-full items-center justify-between rounded-[var(--nova-radius)] px-2.5 py-1.5 text-left ${
                        activeSection === section.id ? 'is-active' : ''
                      }`}
                    >
                      <span className="truncate">{section.title}</span>
                      {expandedSections[section.id] ? (
                        <ChevronUp className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                      ) : (
                        <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                      )}
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </nav>
        </aside>

        <div ref={contentRef} onScroll={onContentScroll} className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 py-5">
          <div className="mx-auto max-w-5xl">
            {sections.map((section) => (
              <Section
                key={section.id}
                ref={(node) => {
                  sectionRefs.current[section.id] = node
                }}
                group={section.group}
                title={section.title}
                expanded={expandedSections[section.id]}
                onToggle={() => toggleSection(section.id)}
              >
                {section.children}
              </Section>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function Section({
  ref,
  group,
  title,
  expanded,
  onToggle,
  children,
}: {
  ref?: (node: HTMLElement | null) => void
  group: string
  title: string
  expanded: boolean
  onToggle: () => void
  children: ReactNode
}) {
  return (
    <section ref={ref} className="scroll-mt-4 border-b border-[var(--nova-border)] py-4 first:pt-0 last:border-b-0">
      <button
        type="button"
        onClick={onToggle}
        className="nova-nav-item mb-2 flex w-full items-center justify-between rounded-[var(--nova-radius)] px-1.5 py-1 text-left"
        aria-expanded={expanded}
      >
        <span className="min-w-0">
          <span className="mr-2 text-[11px] text-[var(--nova-text-faint)]">{group}</span>
          <span className="font-medium text-[var(--nova-text)]">{title}</span>
        </span>
        {expanded ? (
          <ChevronUp className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        ) : (
          <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        )}
      </button>
      {expanded && (
        <div className="nova-settings-section-card space-y-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">{children}</div>
      )}
    </section>
  )
}

function FieldRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="nova-settings-row flex flex-col gap-1.5 rounded-md px-2 py-1.5 sm:flex-row sm:items-center sm:gap-3">
      <span className="w-44 shrink-0 text-[var(--nova-text-muted)]">{label}</span>
      {children}
    </label>
  )
}

function ValueRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="nova-settings-row flex flex-col gap-1.5 rounded-md px-2 py-1.5 sm:flex-row sm:items-center sm:gap-3">
      <span className="w-44 shrink-0 text-[var(--nova-text-muted)]">{label}</span>
      {children}
    </div>
  )
}

function ReadOnly({ label, value }: { label: string; value?: string }) {
  return (
    <ValueRow label={label}>
      <code className="min-h-7 flex-1 truncate rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5 text-[var(--nova-text-muted)]">
        {value || '未设置'}
      </code>
    </ValueRow>
  )
}

function Text({ label, value, placeholder, type = 'text', disabled, onChange }: {
  label: string; value?: string; placeholder?: string; type?: string; disabled?: boolean
  onChange: (v: string) => void
}) {
  return (
    <FieldRow label={label}>
      <input
        type={type}
        value={value ?? ''}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        className={`${fieldCls} disabled:opacity-50`}
      />
    </FieldRow>
  )
}

function Num({ label, value, placeholder, step = 1, min, max, onChange }: {
  label: string; value: number | null; placeholder?: string
  step?: number
  min?: number
  max?: number
  onChange: (v: number | null) => void
}) {
  return (
    <FieldRow label={label}>
      <input
        type="number"
        step={step}
        min={min}
        max={max}
        value={value ?? ''}
        placeholder={placeholder}
        onChange={(e) => {
          const raw = e.target.value
          onChange(raw === '' ? null : Number(raw))
        }}
        className={fieldCls}
      />
    </FieldRow>
  )
}

function BoolTri({ label, value, effective, onChange }: {
  label: string; value: boolean | null; effective?: boolean | null
  onChange: (v: boolean | null) => void
}) {
  const eff = effective === null || effective === undefined ? '未设置' : String(effective)
  return (
    <FieldRow label={label}>
      <select
        value={value === null ? '' : String(value)}
        onChange={(e) => {
          const v = e.target.value
          onChange(v === '' ? null : v === 'true')
        }}
        className={fieldCls}
      >
        <option value="">继承（{eff}）</option>
        <option value="true">开启</option>
        <option value="false">关闭</option>
      </select>
    </FieldRow>
  )
}

function FontSelect({ label, value, effective, onChange }: {
  label: string
  value?: string
  effective?: string
  onChange: (v: string) => void
}) {
  return (
    <FieldRow label={label}>
      <select
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={fieldCls}
      >
        <option value="">继承（{fontLabelFor(effective)}）</option>
        {FONT_OPTIONS.map((font) => (
          <option key={font.value} value={font.value}>{font.label}</option>
        ))}
      </select>
    </FieldRow>
  )
}

function TellerSelect({ label, value, effective, tellers, onChange }: {
  label: string
  value?: string
  effective?: string
  tellers: Teller[]
  onChange: (v: string) => void
}) {
  const effectiveName = tellers.find((teller) => teller.id === effective)?.name || effective || 'classic'
  return (
    <FieldRow label={label}>
      <select
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={fieldCls}
      >
        <option value="">继承（{effectiveName}）</option>
        {tellers.map((teller) => (
          <option key={teller.id} value={teller.id}>{teller.name}</option>
        ))}
      </select>
    </FieldRow>
  )
}

function ModelProfilesEditor({ profiles, effectiveProfiles, onChange }: {
  profiles: ModelProfileSettings[]
  effectiveProfiles: ModelProfileSettings[]
  onChange: (profiles: ModelProfileSettings[]) => void
}) {
  const addProfile = () => {
    const nextIndex = profiles.length + 1
    onChange([...profiles, { id: `model-${nextIndex}`, name: `模型 ${nextIndex}` }])
  }
  const updateProfile = (index: number, patch: Partial<ModelProfileSettings>) => {
    onChange(profiles.map((profile, i) => (i === index ? { ...profile, ...patch } : profile)))
  }
  const removeProfile = (index: number) => {
    onChange(profiles.filter((_, i) => i !== index))
  }

  return (
    <div className="nova-settings-row rounded-md px-2 py-1.5">
      <div className="mb-1.5 text-[var(--nova-text-muted)]">多模型配置</div>
      <div className="flex flex-col gap-2">
        {profiles.length === 0 && (
          <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2 text-[var(--nova-text-faint)]">
            继承 {effectiveProfiles.length || 1} 个模型配置；可新增常见 OpenAI 协议平台。
          </div>
        )}
        {profiles.map((profile, index) => (
          <div key={`${profile.id ?? 'profile'}-${index}`} className="grid gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 md:grid-cols-2">
            <input
              value={profile.id ?? ''}
              placeholder="配置 ID，如 deepseek"
              onChange={(e) => updateProfile(index, { id: e.target.value })}
              className={fieldCls}
            />
            <input
              value={profile.name ?? ''}
              placeholder="显示名称"
              onChange={(e) => updateProfile(index, { name: e.target.value })}
              className={fieldCls}
            />
            <input
              value={profile.openai_base_url ?? ''}
              placeholder="Base URL"
              onChange={(e) => updateProfile(index, { openai_base_url: e.target.value })}
              className={fieldCls}
            />
            <input
              value={profile.openai_model ?? ''}
              placeholder="模型 ID"
              onChange={(e) => updateProfile(index, { openai_model: e.target.value })}
              className={fieldCls}
            />
            <input
              type="password"
              value={profile.openai_api_key ?? ''}
              placeholder="API Key，不填则继承默认"
              onChange={(e) => updateProfile(index, { openai_api_key: e.target.value })}
              className={fieldCls}
            />
            <div className="flex gap-2">
              <input
                type="number"
                step={0.1}
                min={0}
                max={2}
                value={profile.temperature ?? ''}
                placeholder="Temperature，空为平台默认"
                onChange={(e) => updateProfile(index, { temperature: e.target.value === '' ? null : Number(e.target.value) })}
                className={fieldCls}
              />
              <button
                type="button"
                onClick={() => removeProfile(index)}
                className={`${iconButtonCls} shrink-0 border border-[var(--nova-border)] p-1.5`}
                aria-label="删除模型配置"
                title="删除模型配置"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        ))}
        <button
          type="button"
          onClick={addProfile}
          className="nova-nav-item inline-flex w-fit items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-2.5 py-1 text-[var(--nova-text)]"
        >
          <Plus className="h-3.5 w-3.5" />
          添加模型
        </button>
      </div>
    </div>
  )
}
