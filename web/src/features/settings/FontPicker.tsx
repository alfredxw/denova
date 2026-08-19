import { ChevronsUpDown, Laptop, Loader2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import {
  customFontFamilyFromValue,
  customFontValue,
  FONT_OPTIONS,
  fontLabelKeyFor,
  fontStackFor,
  MAX_CUSTOM_FONT_FAMILY_LENGTH,
} from './font-options'
import { queryLocalFontFamilies, type LocalFontQueryResult } from './local-fonts'

type LocalFontStatus = 'idle' | 'loading' | LocalFontQueryResult['status']

interface FontPickerProps {
  id?: string
  value?: string
  inherited?: string
  allowInherit?: boolean
  disabled?: boolean
  className?: string
  onValueChange: (value: string) => void
}

/** Shared preset/local/manual font picker for global settings and editor controls. */
export function FontPicker({
  id,
  value,
  inherited,
  allowInherit = false,
  disabled = false,
  className,
  onValueChange,
}: FontPickerProps) {
  const { t } = useTranslation()
  const generatedID = useId()
  const [open, setOpen] = useState(false)
  const [manualFamily, setManualFamily] = useState('')
  const [localFontStatus, setLocalFontStatus] = useState<LocalFontStatus>('idle')
  const [localFamilies, setLocalFamilies] = useState<string[]>([])

  const displayName = (fontValue?: string) => {
    const customFamily = customFontFamilyFromValue(fontValue)
    if (customFamily) return customFamily
    const labelKey = fontLabelKeyFor(fontValue)
    if (labelKey) return t(labelKey)
    return fontValue?.startsWith('custom:') ? t('settings.font.invalidCustom') : (fontValue || t('common.notSet'))
  }
  const inheritedName = displayName(inherited)
  const selectedName = allowInherit && !value
    ? t('common.inherit', { value: inheritedName })
    : displayName(value)
  const currentCustomFamily = customFontFamilyFromValue(value)
  const manualValue = customFontValue(manualFamily)
  const previewValue = manualValue || value || inherited || 'apple-system'
  const triggerID = id || generatedID
  const customInputID = `${triggerID}-custom`
  const localStatusMessage: Record<LocalFontStatus, string> = {
    idle: t('settings.font.localPrivacy'),
    loading: t('settings.font.localPrivacy'),
    ready: localFamilies.length > 0
      ? t('settings.font.localReady', { count: localFamilies.length })
      : t('settings.font.localEmpty'),
    unsupported: t('settings.font.localUnsupported'),
    denied: t('settings.font.localDenied'),
    failed: t('settings.font.localFailed'),
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) setManualFamily(currentCustomFamily || '')
    setOpen(nextOpen)
  }

  const selectValue = (nextValue: string) => {
    onValueChange(nextValue)
    setOpen(false)
  }

  const loadLocalFonts = async () => {
    setLocalFontStatus('loading')
    const result = await queryLocalFontFamilies()
    setLocalFontStatus(result.status)
    setLocalFamilies(result.families)
  }

  const applyManualFamily = () => {
    if (manualValue) selectValue(manualValue)
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          id={triggerID}
          type="button"
          role="combobox"
          size="sm"
          variant="outline"
          disabled={disabled}
          aria-expanded={open}
          aria-label={t('settings.font.openPicker', { font: selectedName })}
          className={cn('w-full min-w-0 justify-between font-normal', className)}
        >
          <span className="truncate">{selectedName}</span>
          <ChevronsUpDown className="opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={6}
        collisionPadding={8}
        className="nova-panel max-h-[var(--radix-popover-content-available-height)] w-[min(92vw,400px)] gap-0 overflow-y-auto border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
      >
        <Command className="rounded-none bg-transparent">
          <CommandInput placeholder={t('settings.font.searchPlaceholder')} />
          <CommandList className="max-h-[min(42dvh,20rem)]">
            <CommandEmpty>{t('settings.font.noMatches')}</CommandEmpty>
            {allowInherit ? (
              <CommandGroup>
                <CommandItem
                  value={`${t('common.inherit', { value: inheritedName })} inherit`}
                  data-checked={!value || undefined}
                  onSelect={() => selectValue('')}
                >
                  <span className="truncate">{t('common.inherit', { value: inheritedName })}</span>
                </CommandItem>
              </CommandGroup>
            ) : null}
            <CommandGroup heading={t('settings.font.builtinGroup')}>
              {FONT_OPTIONS.map((font) => (
                <CommandItem
                  key={font.value}
                  value={`${t(font.labelKey)} ${font.value}`}
                  data-checked={value === font.value || undefined}
                  onSelect={() => selectValue(font.value)}
                >
                  <span className="truncate" style={{ fontFamily: font.stack }}>{t(font.labelKey)}</span>
                </CommandItem>
              ))}
            </CommandGroup>
            {localFontStatus === 'ready' && localFamilies.length > 0 ? (
              <CommandGroup heading={t('settings.font.localGroup')}>
                {localFamilies.map((family) => {
                  const fontValue = customFontValue(family)
                  return (
                    <CommandItem
                      key={family}
                      value={family}
                      data-checked={currentCustomFamily === family || undefined}
                      onSelect={() => { if (fontValue) selectValue(fontValue) }}
                    >
                      <span className="truncate" style={{ fontFamily: fontStackFor(fontValue, 'apple-system') }}>{family}</span>
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            ) : null}
          </CommandList>
        </Command>

        <div className="space-y-3 border-t border-[var(--nova-border)] p-3">
          <div className="space-y-1.5">
            <div className="flex items-center gap-2">
              {localFontStatus !== 'ready' && localFontStatus !== 'unsupported' ? (
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  disabled={localFontStatus === 'loading'}
                  onClick={loadLocalFonts}
                >
                  {localFontStatus === 'loading' ? <Loader2 className="animate-spin" /> : <Laptop />}
                  {localFontStatus === 'loading'
                    ? t('settings.font.localLoading')
                    : localFontStatus === 'idle'
                      ? t('settings.font.localBrowse')
                      : t('settings.font.localRetry')}
                </Button>
              ) : null}
              <span className="min-w-0 text-[10px] leading-4 text-[var(--nova-text-faint)]" aria-live="polite">
                {localStatusMessage[localFontStatus]}
              </span>
            </div>
          </div>

          <div className="space-y-1.5">
            <label htmlFor={customInputID} className="block text-[10px] font-medium text-[var(--nova-text-muted)]">
              {t('settings.font.customLabel')}
            </label>
            <div className="flex gap-2">
              <Input
                id={customInputID}
                value={manualFamily}
                maxLength={MAX_CUSTOM_FONT_FAMILY_LENGTH}
                aria-invalid={manualFamily.length > 0 && !manualValue}
                placeholder={t('settings.font.customPlaceholder')}
                onChange={(event) => setManualFamily(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key !== 'Enter' || !manualValue) return
                  event.preventDefault()
                  applyManualFamily()
                }}
              />
              <Button type="button" size="sm" disabled={!manualValue} onClick={applyManualFamily}>
                {t('settings.font.customApply')}
              </Button>
            </div>
            <p className="text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('settings.font.customHint')}</p>
          </div>

          <div className="rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
            <div className="text-[9px] uppercase tracking-[0.08em] text-[var(--nova-text-faint)]">{t('settings.font.preview')}</div>
            <div className="mt-1 truncate text-sm text-[var(--nova-text)]" style={{ fontFamily: fontStackFor(previewValue, 'apple-system') }}>
              {t('settings.font.previewText')}
            </div>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
