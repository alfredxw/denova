import { ChevronDown, Laptop, Loader2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { SELECT_TRIGGER_CLASS_NAME } from '@/components/ui/select'
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

const fontSearchIdentity = (value: string) => value.trim().toLocaleLowerCase().replace(/[\s_-]+/g, '')

interface FontPickerProps {
  id?: string
  value?: string
  inherited?: string
  allowInherit?: boolean
  fallback?: string
  disabled?: boolean
  className?: string
  onValueChange: (value: string) => void
}

/** Shared preset, local, and custom font picker for global settings and editor controls. */
export function FontPicker({
  id,
  value,
  inherited,
  allowInherit = false,
  fallback = 'apple-system',
  disabled = false,
  className,
  onValueChange,
}: FontPickerProps) {
  const { t } = useTranslation()
  const generatedID = useId()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
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
  const searchCustomValue = customFontValue(search)
  const searchCustomFamily = customFontFamilyFromValue(searchCustomValue)
  const normalizedSearchFamily = searchCustomFamily ? fontSearchIdentity(searchCustomFamily) : ''
  const hasExactSearchMatch = normalizedSearchFamily
    ? FONT_OPTIONS.some((font) => (
        fontSearchIdentity(font.value) === normalizedSearchFamily
        || fontSearchIdentity(t(font.labelKey)) === normalizedSearchFamily
      )) || localFamilies.some((family) => fontSearchIdentity(family) === normalizedSearchFamily)
    : false
  const currentCustomIsLocal = currentCustomFamily
    ? localFamilies.some((family) => family.toLocaleLowerCase() === currentCustomFamily.toLocaleLowerCase())
    : false
  const customCandidateFamily = searchCustomFamily && !hasExactSearchMatch
    ? searchCustomFamily
    : (!search.trim() && currentCustomFamily && !currentCustomIsLocal ? currentCustomFamily : null)
  const customCandidateValue = customCandidateFamily ? customFontValue(customCandidateFamily) : null
  const triggerID = id || generatedID
  const showLocalAction = localFontStatus !== 'unsupported'
    && !(localFontStatus === 'ready' && localFamilies.length > 0)
  const localFeedback = localFontStatus === 'unsupported'
    ? t('settings.font.localUnsupported')
    : localFontStatus === 'denied'
      ? t('settings.font.localDenied')
      : localFontStatus === 'failed'
        ? t('settings.font.localFailed')
        : localFontStatus === 'ready' && localFamilies.length === 0
          ? t('settings.font.localEmpty')
          : ''

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) setSearch('')
    setOpen(nextOpen)
  }

  const selectValue = (nextValue: string) => {
    onValueChange(nextValue)
    setSearch('')
    setOpen(false)
  }

  const loadLocalFonts = async () => {
    setLocalFontStatus('loading')
    const result = await queryLocalFontFamilies()
    setLocalFontStatus(result.status)
    setLocalFamilies(result.families)
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
          className={cn(
            SELECT_TRIGGER_CLASS_NAME,
            'w-full min-w-0 font-normal hover:bg-transparent aria-expanded:bg-transparent aria-expanded:text-inherit dark:aria-expanded:bg-input/30',
            className,
          )}
        >
          <span className="truncate">{selectedName}</span>
          <ChevronDown className="pointer-events-none size-4 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={6}
        collisionPadding={8}
        className="nova-panel max-h-[var(--radix-popover-content-available-height)] w-[min(92vw,22rem)] gap-0 overflow-hidden border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
      >
        <Command className="rounded-none bg-transparent">
          <CommandInput
            value={search}
            maxLength={MAX_CUSTOM_FONT_FAMILY_LENGTH}
            placeholder={t('settings.font.searchPlaceholder')}
            onValueChange={setSearch}
          />
          <CommandList className="max-h-[min(55dvh,22rem)]">
            <CommandEmpty>{t('settings.font.noMatches')}</CommandEmpty>
            {allowInherit ? (
              <CommandGroup>
                <CommandItem
                  value={`${t('common.inherit', { value: inheritedName })} inherit`}
                  data-checked={!value || undefined}
                  onSelect={() => selectValue('')}
                >
                  <span
                    className="truncate"
                    style={{ fontFamily: fontStackFor(inherited, fallback) }}
                  >
                    {t('common.inherit', { value: inheritedName })}
                  </span>
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
                      data-checked={currentCustomFamily?.toLocaleLowerCase() === family.toLocaleLowerCase() || undefined}
                      onSelect={() => { if (fontValue) selectValue(fontValue) }}
                    >
                      <span className="truncate" style={{ fontFamily: fontStackFor(fontValue, fallback) }}>{family}</span>
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            ) : null}
            {customCandidateFamily && customCandidateValue ? (
              <CommandGroup heading={t('settings.font.customGroup')}>
                <CommandItem
                  value={`${customCandidateFamily} custom`}
                  data-checked={
                    currentCustomFamily?.toLocaleLowerCase() === customCandidateFamily.toLocaleLowerCase() || undefined
                  }
                  onSelect={() => selectValue(customCandidateValue)}
                >
                  <span
                    className="truncate"
                    style={{ fontFamily: fontStackFor(customCandidateValue, fallback) }}
                  >
                    {customCandidateFamily}
                  </span>
                </CommandItem>
              </CommandGroup>
            ) : null}
          </CommandList>
        </Command>

        {showLocalAction || localFeedback ? (
          <div className="border-t border-[var(--nova-border)] p-1.5" aria-live="polite">
            {showLocalAction ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="w-full justify-start text-[var(--nova-text-muted)]"
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
            {localFeedback ? (
              <p className="px-2 py-1 text-[11px] leading-4 text-[var(--nova-text-faint)]">
                {localFeedback}
              </p>
            ) : null}
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}
