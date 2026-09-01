import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { EventPackageModule, StoryDirectorModuleRefs } from '../../types'
import { TuningRow, TuningSelect, type TuningSelectOption, type TuningSource } from './StoryTuningControls'

interface ModuleSelectRowProps {
  label: string
  value: string
  moduleDisabled: boolean
  options: TuningSelectOption[]
  busy: boolean
  disabled: boolean
  locked?: boolean
  onChange: (value: string) => void
}

export function ModuleSelectRow({ label, value, moduleDisabled, options, busy, disabled, locked, onChange }: ModuleSelectRowProps) {
  const visibleOptions = includeIDs(options, value)
  return (
    <TuningRow title={label} source={moduleSelectSource(locked, moduleDisabled)} busy={busy} disabled={disabled}>
      <TuningSelect
        value={value || visibleOptions[0]?.id || ''}
        options={visibleOptions}
        label={label}
        disabled={disabled}
        onChange={onChange}
      />
    </TuningRow>
  )
}

interface EventPackagesRowProps {
  refs: StoryDirectorModuleRefs
  options: EventPackageModule[]
  busy: boolean
  disabled: boolean
  onChange: (ids: string[], disabled: boolean) => void
}

export function EventPackagesRow({ refs, options, busy, disabled, onChange }: EventPackagesRowProps) {
  const { t } = useTranslation()
  const selected = refs.event_package_ids || []
  const visible = includeIDs(
    options.map((item) => ({ id: item.id, label: item.name || item.id })),
    ...selected,
  )
  const summary = refs.event_packages_disabled
    ? t('directorPanel.tuning.disabled')
    : selected.map((id) => optionLabel(visible, id) || id).join(', ') || t('directorPanel.tuning.none')
  return (
    <TuningRow
      title={t('directorPanel.tuning.agent.events')}
      source={refs.event_packages_disabled ? 'off' : 'story'}
      busy={busy}
      disabled={disabled}
    >
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            title={summary}
            className="director-control-select w-[min(10rem,60cqw)] min-w-0 max-w-full justify-between bg-background text-xs font-normal text-foreground"
          >
            <span className="truncate">{summary}</span>
            <ChevronDown data-icon="inline-end" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64">
          <DropdownMenuGroup>
            <DropdownMenuCheckboxItem
              checked={Boolean(refs.event_packages_disabled)}
              onCheckedChange={(checked) => onChange(selected, checked === true)}
            >
              {t('directorPanel.tuning.disabled')}
            </DropdownMenuCheckboxItem>
          </DropdownMenuGroup>
          {visible.length > 0 ? <DropdownMenuSeparator /> : null}
          <DropdownMenuGroup>
            {visible.map((option) => (
              <DropdownMenuCheckboxItem
                key={option.id}
                checked={!refs.event_packages_disabled && selected.includes(option.id)}
                onCheckedChange={(checked) => onChange(
                  checked
                    ? Array.from(new Set([...selected, option.id]))
                    : selected.filter((id) => id !== option.id),
                  false,
                )}
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

function moduleSelectSource(locked: boolean | undefined, disabled: boolean): TuningSource {
  if (locked) return 'locked'
  if (disabled) return 'off'
  return 'story'
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
