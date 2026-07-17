import { useCallback, useMemo, useRef } from 'react'
import type { PointerEvent } from 'react'
import { Check, MessageSquareQuote, Palette, Rows3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export type EditorTheme = 'ide' | 'paper' | 'sepia'

export interface EditorSettings {
  lineHeight: number
  theme: EditorTheme
  dialogueHighlightColor: string
}

export const THEME_STYLES: Record<EditorTheme, { labelKey: string; background: string; color: string; accent: string; dialogueHighlight: string }> = {
  ide: {
    labelKey: 'editor.theme.ide',
    background: 'var(--nova-editor-ide-bg)',
    color: 'var(--nova-editor-ide-color)',
    accent: 'var(--nova-editor-ide-accent)',
    dialogueHighlight: 'var(--nova-dialogue-highlight)',
  },
  paper: {
    labelKey: 'editor.theme.paper',
    background: '#f5efe4',
    color: '#252525',
    accent: '#dfd3c2',
    dialogueHighlight: '#8a3f13',
  },
  sepia: {
    labelKey: 'editor.theme.sepia',
    background: '#efe3cc',
    color: '#2f271f',
    accent: '#d8c6a6',
    dialogueHighlight: '#75451f',
  },
}

const DEFAULT_DIALOGUE_HIGHLIGHT_COLOR = ''
const COLOR_VALUE_PATTERN = /^#[0-9a-fA-F]{6}$/
const DEFAULT_PICKER_COLOR = '#ffd166'

const DEFAULT_SETTINGS: EditorSettings = {
  lineHeight: 1.9,
  theme: 'ide',
  dialogueHighlightColor: DEFAULT_DIALOGUE_HIGHLIGHT_COLOR,
}

export function EditorSettingsPanel({
  settings,
  onChange,
  onClose,
}: {
  settings: EditorSettings
  onChange: (settings: EditorSettings) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const patch = (partial: Partial<EditorSettings>) => onChange({ ...settings, ...partial })

  return (
    <div>
      <div className="border-b border-[var(--nova-border-soft)] px-3 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
              <Palette className="h-3.5 w-3.5" />
            </span>
            <div className="min-w-0">
              <div className="text-xs font-medium text-[var(--nova-text)]">{t('editor.settings')}</div>
              <div className="text-[11px] text-[var(--nova-text-faint)]">{t('editor.settingsDescription')}</div>
            </div>
          </div>
          <button type="button" className="rounded px-2 py-1 text-xs text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>
      </div>

      <div className="space-y-3 p-3">
        <label className="nova-editor-control block rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <div className="mb-2 flex items-center justify-between gap-3 text-xs">
            <span className="flex items-center gap-2 font-medium text-[var(--nova-text-muted)]">
              <Rows3 className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
              {t('editor.lineHeight')}
            </span>
            <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 font-mono text-[11px] text-[var(--nova-text)]">{settings.lineHeight.toFixed(1)}</span>
          </div>
          <input
            type="range"
            min="1.4"
            max="2.6"
            step="0.1"
            value={settings.lineHeight}
            onChange={(e) => patch({ lineHeight: Number(e.target.value) })}
            className="nova-editor-range w-full"
          />
        </label>

        <div className="nova-editor-control block rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <div className="mb-2 flex items-center justify-between gap-3 text-xs">
            <span className="flex items-center gap-2 font-medium text-[var(--nova-text-muted)]">
              <MessageSquareQuote className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
              {t('editor.dialogueHighlightColor')}
            </span>
          </div>
          <DialogueHighlightColorPicker
            value={settings.dialogueHighlightColor}
            defaultColor={THEME_STYLES[settings.theme].dialogueHighlight}
            onChange={(dialogueHighlightColor) => patch({ dialogueHighlightColor })}
            onReset={() => patch({ dialogueHighlightColor: DEFAULT_DIALOGUE_HIGHLIGHT_COLOR })}
          />
        </div>

        <div>
          <div className="mb-2 flex items-center justify-between text-xs text-[var(--nova-text-muted)]">
            <span className="font-medium">{t('editor.backgroundTheme')}</span>
            <span className="text-[11px] text-[var(--nova-text-faint)]">{t('editor.currentTheme', { theme: t(THEME_STYLES[settings.theme].labelKey) })}</span>
          </div>
          <div className="grid gap-2">
            {(Object.keys(THEME_STYLES) as EditorTheme[]).map((theme) => (
              <button
                key={theme}
                type="button"
                className={`nova-editor-theme-option flex w-full items-center justify-between rounded-lg border px-2.5 py-2 text-left text-xs ${
                  settings.theme === theme
                    ? 'is-active border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]'
                    : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                }`}
                onClick={() => patch({ theme })}
              >
                <span className="flex min-w-0 items-center gap-2">
                  <span
                    className="flex h-9 w-12 shrink-0 items-center justify-center rounded-md border border-black/15 text-[10px]"
                    style={{
                      background: THEME_STYLES[theme].background,
                      color: THEME_STYLES[theme].color,
                    }}
                  >
                    Aa
                  </span>
                  <span className="min-w-0">
                    <span className="block font-medium">{t(THEME_STYLES[theme].labelKey)}</span>
                    <span className="mt-0.5 block text-[11px] text-[var(--nova-text-faint)]">{t('editor.themePreview')}</span>
                  </span>
                </span>
                {settings.theme === theme && <Check className="h-3.5 w-3.5 shrink-0 text-[var(--nova-accent-green)]" />}
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

export function loadEditorSettings(): EditorSettings {
  try {
    const raw = localStorage.getItem('nova.editor.settings')
    if (!raw) return DEFAULT_SETTINGS
    const parsed = JSON.parse(raw) as Partial<EditorSettings>
    return {
      lineHeight: parsed.lineHeight ?? DEFAULT_SETTINGS.lineHeight,
      theme: parsed.theme && parsed.theme in THEME_STYLES ? parsed.theme : DEFAULT_SETTINGS.theme,
      dialogueHighlightColor: normalizeColorValue(parsed.dialogueHighlightColor) ?? DEFAULT_SETTINGS.dialogueHighlightColor,
    }
  } catch {
    return DEFAULT_SETTINGS
  }
}

function normalizeColorValue(value: unknown): string | null {
  if (typeof value !== 'string') return null
  if (value === DEFAULT_DIALOGUE_HIGHLIGHT_COLOR) return DEFAULT_DIALOGUE_HIGHLIGHT_COLOR
  return COLOR_VALUE_PATTERN.test(value) ? value : null
}

function DialogueHighlightColorPicker({ value, defaultColor, onChange, onReset }: { value: string; defaultColor: string; onChange: (value: string) => void; onReset: () => void }) {
  const { t } = useTranslation()
  const color = normalizeColorValue(value) || normalizeColorValue(defaultColor) || DEFAULT_PICKER_COLOR
  const hsv = useMemo(() => hexToHsv(color), [color])
  const fieldRef = useRef<HTMLButtonElement>(null)
  const hueRef = useRef<HTMLButtonElement>(null)
  const hueColor = hsvToHex({ h: hsv.h, s: 1, v: 1 })

  const updateFieldColor = useCallback((clientX: number, clientY: number) => {
    const rect = fieldRef.current?.getBoundingClientRect()
    if (!rect) return
    const s = clampNumber((clientX - rect.left) / rect.width, 0, 1)
    const v = clampNumber(1 - ((clientY - rect.top) / rect.height), 0, 1)
    onChange(hsvToHex({ h: hsv.h, s, v }))
  }, [hsv.h, onChange])

  const updateHueColor = useCallback((clientX: number) => {
    const rect = hueRef.current?.getBoundingClientRect()
    if (!rect) return
    const h = clampNumber((clientX - rect.left) / rect.width, 0, 1) * 360
    onChange(hsvToHex({ ...hsv, h }))
  }, [hsv, onChange])

  const handlePointerDrag = (update: (event: PointerEvent<HTMLButtonElement>) => void) => (event: PointerEvent<HTMLButtonElement>) => {
    event.currentTarget.setPointerCapture(event.pointerId)
    update(event)
  }

  const handleHexInput = (raw: string) => {
    const next = raw.startsWith('#') ? raw : `#${raw}`
    if (COLOR_VALUE_PATTERN.test(next)) onChange(next)
  }

  return (
    <div className="space-y-2">
      <button
        ref={fieldRef}
        type="button"
        className="relative h-24 w-full overflow-hidden rounded-md border border-[var(--nova-border)]"
        aria-label={t('editor.dialogueHighlightField')}
        onPointerDown={handlePointerDrag((event) => updateFieldColor(event.clientX, event.clientY))}
        onPointerMove={(event) => { if (event.buttons === 1) updateFieldColor(event.clientX, event.clientY) }}
        style={{
          background: `linear-gradient(to top, #000, transparent), linear-gradient(to right, #fff, ${hueColor})`,
        }}
      >
        <span
          className="absolute h-3 w-3 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-[0_0_0_1px_rgba(0,0,0,0.55)]"
          style={{ left: `${hsv.s * 100}%`, top: `${(1 - hsv.v) * 100}%` }}
        />
      </button>
      <button
        ref={hueRef}
        type="button"
        className="relative h-5 w-full rounded-md border border-[var(--nova-border)]"
        aria-label={t('editor.dialogueHighlightHue')}
        onPointerDown={handlePointerDrag((event) => updateHueColor(event.clientX))}
        onPointerMove={(event) => { if (event.buttons === 1) updateHueColor(event.clientX) }}
        style={{ background: 'linear-gradient(to right, #ef4444, #eab308, #22c55e, #06b6d4, #6366f1, #d946ef, #ef4444)' }}
      >
        <span
          className="absolute top-1/2 h-6 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full border border-white bg-[var(--nova-surface)] shadow-[0_0_0_1px_rgba(0,0,0,0.45)]"
          style={{ left: `${(hsv.h / 360) * 100}%` }}
        />
      </button>
      <div className="flex items-center gap-2">
        <span className="h-7 w-7 shrink-0 rounded-md border border-[var(--nova-border)]" style={{ background: color }} />
        <input
          value={color}
          onChange={(event) => handleHexInput(event.target.value)}
          aria-label={t('editor.dialogueHighlightHex')}
          className="min-w-0 flex-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1 font-mono text-[11px] text-[var(--nova-text)] outline-none focus:border-[var(--nova-field-focus-border)]"
        />
        <button
          type="button"
          className="shrink-0 rounded px-2 py-1 text-[11px] text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={onReset}
        >
          {t('editor.dialogueHighlightReset')}
        </button>
      </div>
    </div>
  )
}

function clampNumber(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function hexToHsv(hex: string) {
  const normalized = normalizeColorValue(hex) || DEFAULT_PICKER_COLOR
  const r = parseInt(normalized.slice(1, 3), 16) / 255
  const g = parseInt(normalized.slice(3, 5), 16) / 255
  const b = parseInt(normalized.slice(5, 7), 16) / 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const delta = max - min
  let h = 0
  if (delta !== 0) {
    if (max === r) h = 60 * (((g - b) / delta) % 6)
    else if (max === g) h = 60 * ((b - r) / delta + 2)
    else h = 60 * ((r - g) / delta + 4)
  }
  if (h < 0) h += 360
  return { h, s: max === 0 ? 0 : delta / max, v: max }
}

function hsvToHex({ h, s, v }: { h: number; s: number; v: number }) {
  const chroma = v * s
  const x = chroma * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = v - chroma
  let r = 0
  let g = 0
  let b = 0
  if (h < 60) [r, g, b] = [chroma, x, 0]
  else if (h < 120) [r, g, b] = [x, chroma, 0]
  else if (h < 180) [r, g, b] = [0, chroma, x]
  else if (h < 240) [r, g, b] = [0, x, chroma]
  else if (h < 300) [r, g, b] = [x, 0, chroma]
  else [r, g, b] = [chroma, 0, x]
  return `#${[r, g, b].map((channel) => Math.round((channel + m) * 255).toString(16).padStart(2, '0')).join('')}`
}
