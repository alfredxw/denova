import { useEffect, useRef, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, ChevronUp, Plus, Save, Settings as SettingsIcon, Trash2, X } from 'lucide-react'
import type { LayeredSettings, Settings, SettingsLayer, StyleRule } from './types'
import { fetchSettings, updateUserSettings, updateWorkspaceSettings } from './api'
import { FONT_OPTIONS, fontLabelFor } from './font-options'
import { getStyles } from '@/lib/api'

type SettingsSectionId = 'model' | 'paths' | 'appearance' | 'agent' | 'ide-editor' | 'ide-style-rules' | 'interactive'

type SettingsSection = {
  id: SettingsSectionId
  group: '公共配置' | 'IDE 模式' | '互动模式'
  title: string
  children: ReactNode
}

const tabCls = 'nova-nav-item rounded-[var(--nova-radius)] px-2.5 py-1 text-xs'
const fieldCls = 'nova-field min-h-7 flex-1 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[#3a3a3a] focus:bg-[var(--nova-surface-3)]'
const iconButtonCls = 'nova-nav-item rounded-[var(--nova-radius)] text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const actionButtonCls = 'nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-1 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

export function SettingsView({ onClose }: { onClose?: () => void }) {
  const [layered, setLayered] = useState<LayeredSettings | null>(null)
  const [activeLayer, setActiveLayer] = useState<SettingsLayer>('user')
  const [draft, setDraft] = useState<Settings>({})
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [availableStyles, setAvailableStyles] = useState<string[]>([])
  const [activeSection, setActiveSection] = useState<SettingsSectionId>('model')
  const [expandedSections, setExpandedSections] = useState<Record<SettingsSectionId, boolean>>({
    model: true,
    paths: true,
    appearance: true,
    agent: true,
    'ide-editor': true,
    'ide-style-rules': true,
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
    // 仅工作区层需要风格列表；切换层时按需加载即可，失败不阻塞主体配置展示。
    if (activeLayer !== 'workspace') return
    getStyles()
      .then((items) => setAvailableStyles(items))
      .catch((e) => console.warn('[settings] 获取风格参考列表失败', e))
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
          <FontSelect label="阅读字体" value={draft.reading_font_family}
                      effective={effective.reading_font_family}
                      onChange={(v) => setField('reading_font_family', v)} />
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
        </>
      ),
    },
    ...(activeLayer === 'workspace'
      ? [{
          id: 'ide-style-rules' as const,
          group: 'IDE 模式' as const,
          title: '场景化风格规则',
          children: (
            <StyleRulesEditor
              available={availableStyles}
              rules={draft.style_rules ?? []}
              effective={effective.style_rules ?? []}
              onChange={(v) => setField('style_rules', v)}
            />
          ),
        }]
      : []),
    {
      id: 'interactive',
      group: '互动模式',
      title: '故事舞台',
      children: activeLayer === 'workspace' ? (
        <>
          <Num label="单轮目标字数" value={draft.interactive_reply_target_chars ?? null}
               placeholder={placeholderFor('interactive_reply_target_chars')}
               onChange={(v) => setField('interactive_reply_target_chars', v)} />
          <Num label="最大输出 Token" value={draft.interactive_max_tokens ?? null}
               placeholder="不填则不限制，优先避免截断"
               onChange={(v) => setField('interactive_max_tokens', v)} />
          <Num label="故事舞台字号 (px)" value={draft.interactive_stage_font_size ?? null}
               placeholder={placeholderFor('interactive_stage_font_size')}
               onChange={(v) => setField('interactive_stage_font_size', v)} />
          <Num label="故事舞台行间距" value={draft.interactive_stage_line_height ?? null}
               placeholder={placeholderFor('interactive_stage_line_height')}
               step={0.05}
               onChange={(v) => setField('interactive_stage_line_height', v)} />
        </>
      ) : (
        <>
          <ReadOnly label="单轮目标字数" value={`当前工作区配置，生效值：${effective.interactive_reply_target_chars ?? 1200} 个中文字`} />
          <Num label="故事舞台字号 (px)" value={draft.interactive_stage_font_size ?? null}
               placeholder={placeholderFor('interactive_stage_font_size')}
               onChange={(v) => setField('interactive_stage_font_size', v)} />
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
    <div className="nova-sidebar flex h-full min-h-0 w-full flex-col text-[var(--nova-text)]">
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

      {error && <div className="border-b border-red-500/40 bg-red-500/10 px-4 py-1.5 text-xs text-red-400">{error}</div>}

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
        <span>
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
        <div className="space-y-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.035)]">{children}</div>
      )}
    </section>
  )
}

function FieldRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
      <span className="w-44 shrink-0 text-[var(--nova-text-muted)]">{label}</span>
      {children}
    </label>
  )
}

function ValueRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
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

function Num({ label, value, placeholder, step = 1, onChange }: {
  label: string; value: number | null; placeholder?: string
  step?: number
  onChange: (v: number | null) => void
}) {
  return (
    <FieldRow label={label}>
      <input
        type="number"
        step={step}
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

function StyleRulesEditor({ available, rules, effective, onChange }: {
  available: string[]
  rules: StyleRule[]
  effective: StyleRule[]
  onChange: (v: StyleRule[]) => void
}) {
  const addRule = () => onChange([...rules, { scene: '', styles: [] }])
  const removeRule = (idx: number) => onChange(rules.filter((_, i) => i !== idx))
  const updateRule = (idx: number, patch: Partial<StyleRule>) =>
    onChange(rules.map((r, i) => (i === idx ? { ...r, ...patch } : r)))
  const inheriting = rules.length === 0 && effective.length > 0

  return (
    <div className="flex flex-col gap-3">
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[var(--nova-text-muted)]">
        为不同场景配置不同的风格参考。Agent 在创作章节正文时，会根据本轮要写的内容自动匹配最贴近的场景，并 read_file 读取对应风格文件作为文风参考。
        本轮通过 # 显式指定风格则优先使用本轮指定，忽略此处规则。
      </div>

      {inheriting && (
        <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[var(--nova-text-faint)]">
          继承生效（{effective.length} 条规则）：
          <ul className="mt-1 space-y-0.5">
            {effective.map((r, i) => (
              <li key={i}>
                <span className="text-[var(--nova-text-muted)]">{r.scene || '（未命名场景）'}</span>
                <span className="ml-2">→ {r.styles.join('、') || '（无风格文件）'}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {rules.length > 0 && (
        <div className="space-y-2">
          {rules.map((rule, idx) => (
            <StyleRuleRow
              key={idx}
              available={available}
              rule={rule}
              onChange={(patch) => updateRule(idx, patch)}
              onRemove={() => removeRule(idx)}
            />
          ))}
        </div>
      )}

      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={addRule}
          className={actionButtonCls}
        >
          <Plus className="h-3.5 w-3.5" />
          新增规则
        </button>
        {available.length === 0 && (
          <span className="text-[var(--nova-text-faint)]">提示：当前工作区 setting/styles/ 下尚无任何风格文件。</span>
        )}
      </div>
    </div>
  )
}

function StyleRuleRow({ available, rule, onChange, onRemove }: {
  available: string[]
  rule: StyleRule
  onChange: (patch: Partial<StyleRule>) => void
  onRemove: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const toggleStyle = (path: string) => {
    if (rule.styles.includes(path)) {
      onChange({ styles: rule.styles.filter((p) => p !== path) })
    } else {
      onChange({ styles: [...rule.styles, path] })
    }
  }
  const summary = rule.styles.length === 0 ? '尚未选择风格文件' : rule.styles.join('、')

  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <input
          type="text"
          value={rule.scene}
          placeholder="场景描述（如：激烈打斗 / 日常对话 / 宏大世界观铺陈）"
          onChange={(e) => onChange({ scene: e.target.value })}
          className={fieldCls}
        />
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className={`${actionButtonCls} justify-center`}
          title={expanded ? '收起' : '展开选择风格'}
        >
          {expanded ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          {expanded ? '收起' : `风格 (${rule.styles.length})`}
        </button>
        <button
          type="button"
          onClick={onRemove}
          className={`${actionButtonCls} justify-center hover:bg-red-500/15 hover:text-red-200`}
          title="删除规则"
        >
          <Trash2 className="h-3.5 w-3.5" />
          删除
        </button>
      </div>

      {!expanded && (
        <div className="mt-1 truncate px-1 text-[var(--nova-text-faint)]">→ {summary}</div>
      )}

      {expanded && (
        <div className="mt-2 max-h-48 overflow-y-auto rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
          {available.length === 0 ? (
            <div className="px-2 py-2 text-[var(--nova-text-faint)]">无可用风格文件</div>
          ) : (
            available.map((path) => (
              <label key={path} className="flex cursor-pointer items-center gap-2 px-2 py-1.5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]">
                <input
                  type="checkbox"
                  checked={rule.styles.includes(path)}
                  onChange={() => toggleStyle(path)}
                />
                <span className="truncate">{path}</span>
              </label>
            ))
          )}
        </div>
      )}
    </div>
  )
}
