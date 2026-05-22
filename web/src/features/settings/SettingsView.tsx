import { useEffect, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import type { LayeredSettings, Settings, SettingsLayer } from './types'
import { fetchSettings, updateUserSettings, updateWorkspaceSettings } from './api'

const NOVA_DIR_ONLY_USER: (keyof Settings)[] = ['nova_dir']

export function SettingsView() {
  const [layered, setLayered] = useState<LayeredSettings | null>(null)
  const [activeLayer, setActiveLayer] = useState<SettingsLayer>('user')
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

  const onSave = async () => {
    setSaving(true)
    setError(null)
    try {
      const updater = activeLayer === 'user' ? updateUserSettings : updateWorkspaceSettings
      const next = await updater(draft)
      setLayered(next)
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

  const isDisabled = (k: keyof Settings) =>
    activeLayer === 'workspace' && NOVA_DIR_ONLY_USER.includes(k)

  return (
    <div className="flex h-full flex-col bg-[#1b1c1f] text-[#d7dbe2]">
      <div className="flex h-9 items-center gap-1 border-b border-[#303238] bg-[#202124] px-3 text-xs">
        <span className="font-medium text-[#c5c9d1]">设置</span>
        <div className="ml-4 flex gap-1">
          {(['user', 'workspace'] as SettingsLayer[]).map((l) => (
            <button
              key={l}
              onClick={() => setActiveLayer(l)}
              className={`rounded px-2 py-0.5 ${
                activeLayer === l ? 'bg-[#2f7dd3] text-white' : 'text-[#9aa0aa] hover:bg-[#303238]'
              }`}
            >
              {l === 'user' ? '用户配置' : '当前工作区'}
            </button>
          ))}
        </div>
        <button
          onClick={onSave}
          disabled={saving}
          className="ml-auto rounded bg-[#0e639c] px-3 py-0.5 text-white disabled:opacity-50"
        >
          {saving ? '保存中…' : '保存'}
        </button>
      </div>

      {error && <div className="border-b border-red-500/40 bg-red-500/10 px-3 py-1 text-xs text-red-400">{error}</div>}

      <div className="flex-1 overflow-y-auto p-4 text-xs">
        <Section title="模型">
          <Text label="API Key" value={draft.openai_api_key} placeholder={placeholderFor('openai_api_key')}
                onChange={(v) => setField('openai_api_key', v)} type="password" />
          <Text label="Base URL" value={draft.openai_base_url} placeholder={placeholderFor('openai_base_url')}
                onChange={(v) => setField('openai_base_url', v)} />
          <Text label="模型" value={draft.openai_model} placeholder={placeholderFor('openai_model')}
                onChange={(v) => setField('openai_model', v)} />
        </Section>

        <Section title="路径">
          <Text label="Skills 目录" value={draft.skills_dir} placeholder={placeholderFor('skills_dir')}
                onChange={(v) => setField('skills_dir', v)} />
          <Text label="用户数据目录 (nova_dir)" value={draft.nova_dir}
                placeholder={isDisabled('nova_dir') ? '仅可在用户配置中修改' : placeholderFor('nova_dir')}
                disabled={isDisabled('nova_dir')}
                onChange={(v) => setField('nova_dir', v)} />
        </Section>

        <Section title="编辑器">
          <BoolTri label="自动保存" value={draft.auto_save_enabled ?? null}
                   effective={effective.auto_save_enabled}
                   onChange={(v) => setField('auto_save_enabled', v)} />
          <Num label="自动保存间隔 (ms)" value={draft.auto_save_interval_ms ?? null}
               placeholder={placeholderFor('auto_save_interval_ms')}
               onChange={(v) => setField('auto_save_interval_ms', v)} />
          <Text label="章节文件名模板" value={draft.chapter_filename_format}
                placeholder={placeholderFor('chapter_filename_format')}
                onChange={(v) => setField('chapter_filename_format', v)} />
        </Section>

        <Section title="Agent">
          <Num label="最大迭代轮数" value={draft.max_iteration ?? null}
               placeholder={placeholderFor('max_iteration')}
               onChange={(v) => setField('max_iteration', v)} />
          <Num label="模型重试次数" value={draft.model_max_retries ?? null}
               placeholder={placeholderFor('model_max_retries')}
               onChange={(v) => setField('model_max_retries', v)} />
          <BoolTri label="默认 PlanMode" value={draft.plan_mode_default ?? null}
                   effective={effective.plan_mode_default}
                   onChange={(v) => setField('plan_mode_default', v)} />
        </Section>
      </div>
    </div>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="mb-6">
      <div className="mb-2 text-[11px] font-medium uppercase tracking-wide text-[#7f8590]">{title}</div>
      <div className="space-y-2 rounded border border-[#303238] bg-[#202124] p-3">{children}</div>
    </div>
  )
}

function Text({ label, value, placeholder, type = 'text', disabled, onChange }: {
  label: string; value?: string; placeholder?: string; type?: string; disabled?: boolean
  onChange: (v: string) => void
}) {
  return (
    <label className="flex items-center gap-3">
      <span className="w-44 shrink-0 text-[#9aa0aa]">{label}</span>
      <input
        type={type}
        value={value ?? ''}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        className="flex-1 rounded border border-[#303238] bg-[#1b1c1f] px-2 py-1 text-[#d7dbe2] disabled:opacity-50"
      />
    </label>
  )
}

function Num({ label, value, placeholder, onChange }: {
  label: string; value: number | null; placeholder?: string
  onChange: (v: number | null) => void
}) {
  return (
    <label className="flex items-center gap-3">
      <span className="w-44 shrink-0 text-[#9aa0aa]">{label}</span>
      <input
        type="number"
        value={value ?? ''}
        placeholder={placeholder}
        onChange={(e) => {
          const raw = e.target.value
          onChange(raw === '' ? null : Number(raw))
        }}
        className="flex-1 rounded border border-[#303238] bg-[#1b1c1f] px-2 py-1 text-[#d7dbe2]"
      />
    </label>
  )
}

function BoolTri({ label, value, effective, onChange }: {
  label: string; value: boolean | null; effective?: boolean | null
  onChange: (v: boolean | null) => void
}) {
  const eff = effective === null || effective === undefined ? '未设置' : String(effective)
  return (
    <label className="flex items-center gap-3">
      <span className="w-44 shrink-0 text-[#9aa0aa]">{label}</span>
      <select
        value={value === null ? '' : String(value)}
        onChange={(e) => {
          const v = e.target.value
          onChange(v === '' ? null : v === 'true')
        }}
        className="flex-1 rounded border border-[#303238] bg-[#1b1c1f] px-2 py-1 text-[#d7dbe2]"
      >
        <option value="">继承（{eff}）</option>
        <option value="true">开启</option>
        <option value="false">关闭</option>
      </select>
    </label>
  )
}
