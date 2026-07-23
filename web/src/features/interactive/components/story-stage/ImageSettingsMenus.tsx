import { useEffect, useMemo, useState } from 'react'
import { Check, ImagePlus, Loader2, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from '@/components/ui/dropdown-menu'
import type { ImagePreset, StoryImageSettings, StorySummary } from '../../types'

const DEFAULT_IMAGE_INTERVAL_TURNS = 3

export function InteractiveImageSettingsMenu({ story, disabled, onChange }: { story?: StorySummary; disabled?: boolean; onChange?: (settings: StoryImageSettings) => void | Promise<void> }) {
  const { t } = useTranslation()
  const current = normalizeStoryImageSettings(story?.image_settings)
  const [intervalDraft, setIntervalDraft] = useState(String(current.interval_turns))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    setIntervalDraft(String(current.interval_turns))
    setError('')
  }, [current.interval_turns, current.mode])

  const save = async (patch: Partial<StoryImageSettings>) => {
    if (disabled || !onChange) return
    const next = normalizeStoryImageSettings({ ...current, ...patch })
    setSaving(true)
    setError('')
    try {
      await onChange(next)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyStage.interactiveImage.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const saveInterval = () => {
    const intervalTurns = normalizeIntervalTurns(intervalDraft)
    setIntervalDraft(String(intervalTurns))
    void save({ mode: 'interval', interval_turns: intervalTurns })
  }

  return (
    <>
      <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
      <DropdownMenuSub>
        <DropdownMenuSubTrigger
          disabled={disabled}
          className="flex cursor-pointer items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
        >
          <span className="flex h-3.5 w-3.5 items-center justify-center">
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--nova-text-faint)]" /> : <ImagePlus className="h-3.5 w-3.5" />}
          </span>
          <span className="min-w-0 flex-1 truncate">{t('storyStage.interactiveImage.menuTitle')}</span>
          <span className="max-w-36 shrink-0 truncate text-right text-[10px] text-[var(--nova-text-faint)]">{imageSettingsSummary(current, t)}</span>
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent className="w-72 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
          <DropdownMenuItem
            disabled={disabled || saving}
            onSelect={(event) => {
              event.preventDefault()
              void save({ mode: 'manual', interval_turns: current.interval_turns })
            }}
            onClick={() => void save({ mode: 'manual', interval_turns: current.interval_turns })}
            className="grid cursor-pointer grid-cols-[1rem_minmax(0,1fr)] items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
          >
            <Check className={`h-3.5 w-3.5 ${current.mode === 'manual' ? 'opacity-100' : 'opacity-0'}`} />
            <span className="min-w-0 flex-1 truncate">{t('storyStage.interactiveImage.modeManual')}</span>
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={disabled || saving}
            onSelect={(event) => {
              event.preventDefault()
              saveInterval()
            }}
            onClick={saveInterval}
            className="grid cursor-pointer grid-cols-[1rem_minmax(0,1fr)] items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
          >
            <Check className={`h-3.5 w-3.5 ${current.mode === 'interval' ? 'opacity-100' : 'opacity-0'}`} />
            <span className="min-w-0 flex-1 truncate">{t('storyStage.interactiveImage.modeInterval', { count: normalizeIntervalTurns(intervalDraft) })}</span>
          </DropdownMenuItem>
          <div className="mt-1 grid grid-cols-[1rem_minmax(0,1fr)_auto] items-center gap-2 px-2 py-1.5">
            <span />
            <div className="text-[11px] text-[var(--nova-text-faint)]">{t('storyStage.interactiveImage.intervalLabel')}</div>
            <div className="flex items-center justify-end gap-2">
              <Input
                aria-label={t('storyStage.interactiveImage.intervalInputLabel')}
                className="nova-field h-7 w-16 text-center text-xs"
                type="number"
                min={1}
                max={50}
                disabled={disabled || saving}
                value={intervalDraft}
                onPointerDown={(event) => event.stopPropagation()}
                onChange={(event) => {
                  setIntervalDraft(event.target.value)
                  setError('')
                }}
                onKeyDown={(event) => {
                  event.stopPropagation()
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    saveInterval()
                  }
                }}
                onBlur={saveInterval}
              />
              <span className="text-[11px] text-[var(--nova-text-faint)]">{t('storyStage.interactiveImage.intervalSuffix')}</span>
            </div>
            {error ? <div className="col-span-3 text-[11px] leading-4 text-[var(--nova-danger)]">{error}</div> : null}
          </div>
        </DropdownMenuSubContent>
      </DropdownMenuSub>
    </>
  )
}

export function StoryImagePresetMenu({ story, presets, disabled, onChange }: { story?: StorySummary; presets: ImagePreset[]; disabled?: boolean; onChange?: (settings: StoryImageSettings) => void | Promise<void> }) {
  const { t } = useTranslation()
  const current = normalizeStoryImageSettings(story?.image_settings)
  const [saving, setSaving] = useState(false)
  const normalizedPresets = useMemo(() => {
    if (presets.some((preset) => preset.id === current.preset_id)) return presets
    return [{ id: current.preset_id || 'game-cg', name: current.preset_id || 'game-cg', description: '', prompt: '', custom: true, version: 1 }, ...presets]
  }, [current.preset_id, presets])
  const selected = normalizedPresets.find((preset) => preset.id === current.preset_id) || normalizedPresets.find((preset) => preset.id === 'game-cg') || normalizedPresets[0]

  const save = async (presetId: string) => {
    if (disabled || !onChange || saving || presetId === current.preset_id) return
    setSaving(true)
    try {
      await onChange(normalizeStoryImageSettings({ ...current, preset_id: presetId }))
    } catch (err) {
      console.warn('[interactive-stage] 保存图像方案失败', err)
    } finally {
      setSaving(false)
    }
  }

  if (normalizedPresets.length === 0) return null

  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger
        disabled={disabled || saving}
        className="flex cursor-pointer items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
      >
        <span className="flex h-3.5 w-3.5 items-center justify-center">
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--nova-text-faint)]" /> : <Sparkles className="h-3.5 w-3.5" />}
        </span>
        <span className="min-w-0 flex-1 truncate">{t('storyStage.imagePreset.menuTitle')}</span>
        <span className="max-w-36 shrink-0 truncate text-right text-[10px] text-[var(--nova-text-faint)]">{selected?.name || current.preset_id}</span>
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="w-72 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
        {normalizedPresets.map((preset) => {
          const selectedPreset = preset.id === selected?.id
          return (
            <DropdownMenuItem
              key={preset.id}
              disabled={disabled || saving}
              onSelect={(event) => {
                event.preventDefault()
                void save(preset.id)
              }}
              onClick={() => void save(preset.id)}
              className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
            >
              <Check className={`h-3.5 w-3.5 ${selectedPreset ? 'opacity-100' : 'opacity-0'}`} />
              <span className="min-w-0 flex-1 truncate">{preset.name || preset.id}</span>
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  )
}

function normalizeStoryImageSettings(value?: Partial<StoryImageSettings> | null): StoryImageSettings {
  const rawMode = typeof value?.mode === 'string' ? String(value.mode) : ''
  const mode = rawMode === 'interval' || rawMode === 'every_turn' ? 'interval' : 'manual'
  return {
    mode,
    interval_turns: rawMode === 'every_turn' ? 1 : normalizeIntervalTurns(value?.interval_turns),
    preset_id: typeof value?.preset_id === 'string' && value.preset_id.trim() ? value.preset_id.trim() : 'game-cg',
  }
}

function normalizeIntervalTurns(value: unknown) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue) || numberValue <= 0) return DEFAULT_IMAGE_INTERVAL_TURNS
  return Math.min(50, Math.max(1, Math.floor(numberValue)))
}

function imageSettingsSummary(settings: StoryImageSettings, t: (key: string, options?: Record<string, unknown>) => string) {
  if (settings.mode === 'interval') return t('storyStage.interactiveImage.currentInterval', { count: settings.interval_turns })
  return t('storyStage.interactiveImage.currentManual')
}
