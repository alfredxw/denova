import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { EventPackageModule, StoryDirectorModuleRefs } from '../../types'
import { TuningRow, TuningSelect, type TuningSelectOption, type TuningSource } from './StoryTuningControls'

export const PRESET_VALUE = '__preset__'

interface ModuleSelectRowProps {
  label: string
  description?: string
  value: string
  baseValue: string
  moduleDisabled: boolean
  options: TuningSelectOption[]
  busy: boolean
  disabled: boolean
  locked?: boolean
  onChange: (value: string) => void
}

export function ModuleSelectRow({ label, description, value, baseValue, moduleDisabled, options, busy, disabled, locked, onChange }: ModuleSelectRowProps) {
  const { t } = useTranslation()
  const visibleOptions = includeIDs(options, baseValue, value)
  const source = moduleSelectSource(locked, moduleDisabled, value === baseValue)
  const selectValue = value === baseValue ? PRESET_VALUE : value
  const baseLabel = optionLabel(visibleOptions, baseValue) || baseValue || t('directorPanel.tuning.none')
  return (
    <TuningRow title={label} description={description} source={source} busy={busy} disabled={disabled}>
      <TuningSelect
        value={selectValue || PRESET_VALUE}
        options={[{ id: PRESET_VALUE, label: t('directorPanel.tuning.usePreset', { value: baseLabel }) }, ...visibleOptions]}
        label={label}
        disabled={disabled}
        onChange={onChange}
      />
    </TuningRow>
  )
}

interface EventPackagesRowProps {
  refs: StoryDirectorModuleRefs
  presetRefs: StoryDirectorModuleRefs
  options: EventPackageModule[]
  busy: boolean
  disabled: boolean
  onChange: (ids: string[], disabled: boolean) => void
}

export function EventPackagesRow({ refs, presetRefs, options, busy, disabled, onChange }: EventPackagesRowProps) {
  const { t } = useTranslation()
  const selected = refs.event_package_ids || []
  const preset = presetRefs.event_package_ids || []
  const visible = includeIDs(options.map((item) => ({ id: item.id, label: item.name || item.id })), ...preset, ...selected)
  const source = eventPackagesSource(refs, selected, preset)
  const summary = refs.event_packages_disabled
    ? t('directorPanel.tuning.disabled')
    : selected.map((id) => optionLabel(visible, id) || id).join(', ') || t('directorPanel.tuning.none')
  return (
    <TuningRow title={t('directorPanel.tuning.agent.events')} source={source} busy={busy} disabled={disabled}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button type="button" variant="outline" size="sm" disabled={disabled} className="w-36 min-w-0 max-w-full justify-between bg-background text-xs font-normal text-foreground">
            <span className="truncate">{summary}</span>
            <ChevronDown data-icon="inline-end" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64">
          <DropdownMenuGroup>
            <DropdownMenuItem onSelect={() => onChange([...preset], Boolean(presetRefs.event_packages_disabled))}>{t('directorPanel.tuning.usePreset', { value: preset.map((id) => optionLabel(visible, id) || id).join(', ') || t('directorPanel.tuning.none') })}</DropdownMenuItem>
            <DropdownMenuCheckboxItem checked={Boolean(refs.event_packages_disabled)} onCheckedChange={(checked) => onChange(selected, checked === true)}>{t('directorPanel.tuning.disabled')}</DropdownMenuCheckboxItem>
          </DropdownMenuGroup>
          {visible.length > 0 ? <DropdownMenuSeparator /> : null}
          <DropdownMenuGroup>
            {visible.map((option) => (
              <DropdownMenuCheckboxItem
                key={option.id}
                checked={!refs.event_packages_disabled && selected.includes(option.id)}
                onCheckedChange={(checked) => onChange(checked ? Array.from(new Set([...selected, option.id])) : selected.filter((id) => id !== option.id), false)}
              >
                {option.label}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </TuningRow>
  )
}

function moduleSelectSource(locked: boolean | undefined, disabled: boolean, inherited: boolean): TuningSource {
  if (locked) return 'locked'
  if (disabled) return 'off'
  return inherited ? 'preset' : 'story'
}

function eventPackagesSource(refs: StoryDirectorModuleRefs, selected: string[], preset: string[]): TuningSource {
  if (refs.event_packages_disabled) return 'off'
  return arraysEqual(selected, preset) ? 'preset' : 'story'
}

function includeIDs(options: TuningSelectOption[], ...ids: string[]): TuningSelectOption[] {
  const result = [...options]
  for (const id of ids) {
    if (id && !result.some((option) => option.id === id)) result.push({ id, label: id })
  }
  return result
}

function optionLabel(options: TuningSelectOption[], id: string | undefined): string {
  return options.find((option) => option.id === id)?.label || id || ''
}

function arraysEqual(left: string[], right: string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index])
}
