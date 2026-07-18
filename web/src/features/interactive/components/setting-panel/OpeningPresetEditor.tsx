import { Plus, Sparkles, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isSaveShortcut } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { newBookOpeningPreset, type BookOpeningPreset } from '../../opening'
import { EmptyState, Field, iconActionClassName, inputClassName } from './editor-shared'

export function OpeningPresetEditor({
  presets,
  activeId,
  setActiveId,
  setPresets,
  onSave,
}: {
  presets: BookOpeningPreset[]
  activeId: string
  setActiveId: (id: string) => void
  setPresets: (presets: BookOpeningPreset[]) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  const activePreset = presets.find((preset) => preset.id === activeId) || presets[0] || null
  const updateActivePreset = (patch: Partial<BookOpeningPreset>) => {
    if (!activePreset) return
    setPresets(presets.map((preset) => (preset.id === activePreset.id ? { ...preset, ...patch } : preset)))
  }
  const addPreset = () => {
    const preset = newBookOpeningPreset(t('settingPanel.openingPreset.defaultName', { number: presets.length + 1 }))
    setPresets([...presets, preset])
    setActiveId(preset.id)
  }
  const deleteActivePreset = () => {
    if (!activePreset) return
    const nextPresets = presets.filter((preset) => preset.id !== activePreset.id)
    setPresets(nextPresets)
    setActiveId(nextPresets[0]?.id || '')
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto md:overflow-hidden">
      <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.openingPreset.title')}</div>
            <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.openingPreset.description')}</div>
          </div>
          <Button className={iconActionClassName} variant="outline" size="sm" onClick={addPreset}>
            <Plus className="h-3.5 w-3.5" />
            {t('settingPanel.openingPreset.add')}
          </Button>
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <aside className="max-h-48 shrink-0 overflow-y-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-2 md:max-h-none md:w-56 md:border-b-0 md:border-r">
          {presets.length === 0 ? (
            <div className="px-2 py-3 text-xs leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.openingPreset.empty')}</div>
          ) : (
            <div className="space-y-1">
              {presets.map((preset) => (
                <button
                  key={preset.id}
                  type="button"
                  onClick={() => setActiveId(preset.id)}
                  className={`flex min-h-8 w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs transition ${
                    activePreset?.id === preset.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                  }`}
                >
                  <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                  <span className="min-w-0 flex-1 truncate">{preset.title || t('settingPanel.openingPreset.untitled')}</span>
                </button>
              ))}
            </div>
          )}
        </aside>
        <div className="min-h-[420px] flex-1 p-4 md:min-h-0">
          {activePreset ? (
            <div className="flex h-full min-h-0 flex-col gap-3">
              <div className="flex items-end gap-3">
                <Field className="min-w-0 flex-1" label={t('settingPanel.openingPreset.name')}>
                  <Input className={inputClassName} value={activePreset.title} onChange={(event) => updateActivePreset({ title: event.target.value })} placeholder={t('settingPanel.openingPreset.untitled')} />
                </Field>
                <Button className={iconActionClassName} variant="outline" size="icon" onClick={deleteActivePreset} aria-label={t('settingPanel.openingPreset.delete')}>
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
              <Textarea
                autoResize={false}
                className="nova-field min-h-0 flex-1 resize-none text-sm leading-7 shadow-none focus-visible:ring-0"
                value={activePreset.content}
                onChange={(event) => updateActivePreset({ content: event.target.value })}
                placeholder={t('settingPanel.openingPreset.placeholder')}
                onKeyDown={(event) => {
                  if (isSaveShortcut(event)) {
                    event.preventDefault()
                    event.stopPropagation()
                    onSave()
                  }
                }}
              />
            </div>
          ) : (
            <EmptyState title={t('settingPanel.openingPreset.emptyTitle')} description={t('settingPanel.openingPreset.emptyDesc')} />
          )}
        </div>
      </div>
    </div>
  )
}
