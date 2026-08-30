import { useEffect, useState, type ReactNode } from 'react'
import { Loader2, Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldContent, FieldDescription, FieldError, FieldGroup, FieldTitle } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

export type TuningSource = 'preset' | 'story' | 'off' | 'locked'

export function ControlSection({
  icon,
  title,
  action,
  children,
}: {
  icon: ReactNode
  title: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="overflow-hidden rounded-xl border border-border bg-card">
      <header className="flex min-h-9 items-center gap-2 px-2.5 py-1.5">
        <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-muted text-[var(--director-brass)]">{icon}</span>
        <h3 className="min-w-0 flex-1 truncate text-xs font-semibold text-foreground">{title}</h3>
        {action ? <div className="director-control-section__action shrink-0">{action}</div> : null}
      </header>
      <FieldGroup className="gap-0 border-t border-border">{children}</FieldGroup>
    </section>
  )
}

export function TuningRow({
  title,
  description,
  source,
  busy,
  disabled,
  children,
}: {
  title: string
  description?: string
  source?: TuningSource
  busy?: boolean
  disabled?: boolean
  children: ReactNode
}) {
  return (
    <Field orientation="horizontal" data-disabled={disabled || undefined} className="director-control-row min-w-0 items-center gap-1.5 px-2.5 py-2 [&:not(:last-child)]:border-b [&:not(:last-child)]:border-border">
      <FieldContent className="min-w-0 overflow-hidden">
        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
          <FieldTitle className="max-w-full truncate whitespace-nowrap text-xs" title={title}>{title}</FieldTitle>
          {source && source !== 'preset' ? <TuningSourceBadge source={source} /> : null}
          {busy ? <Loader2 className="size-3 animate-spin text-muted-foreground" aria-label={title} /> : null}
        </div>
        {description ? <FieldDescription className="text-[10px] leading-4">{description}</FieldDescription> : null}
      </FieldContent>
      <div className="director-control-row__value flex min-w-0 shrink-0 items-center justify-end">{children}</div>
    </Field>
  )
}

export function TuningSourceBadge({ source }: { source: TuningSource }) {
  const { t } = useTranslation()
  return (
    <Badge variant={source === 'off' ? 'outline' : 'secondary'} className="h-4 px-1.5 text-[9px] font-medium">
      {t(`directorPanel.tuning.source.${source}`)}
    </Badge>
  )
}

export interface TuningSelectOption {
  id: string
  label: string
}

export function TuningSelect({
  value,
  options,
  label,
  disabled,
  onChange,
}: {
  value: string
  options: TuningSelectOption[]
  label: string
  disabled?: boolean
  onChange: (value: string) => void
}) {
  return (
    <Select value={value} disabled={disabled} onValueChange={onChange}>
      <SelectTrigger size="sm" aria-label={label} className="director-control-select w-32 min-w-0 max-w-full shrink-0 bg-background text-xs text-foreground">
        <SelectValue />
      </SelectTrigger>
      <SelectContent position="popper">
        <SelectGroup>
          {options.map((option) => <SelectItem key={option.id} value={option.id}>{option.label}</SelectItem>)}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

export function NumberSettingInput({
  value,
  min,
  max,
  label,
  disabled,
  onCommit,
}: {
  value: number
  min: number
  max?: number
  label: string
  disabled?: boolean
  onCommit: (value: number) => void
}) {
  const [draft, setDraft] = useState(String(value))
  const parsed = Number(draft)
  const valid = Number.isInteger(parsed) && parsed >= min && (max === undefined || parsed <= max)

  useEffect(() => setDraft(String(value)), [value])

  const commit = () => {
    if (!valid) return
    const next = Math.trunc(parsed)
    setDraft(String(next))
    if (next !== value) onCommit(next)
  }

  return (
    <div className="flex min-w-0 flex-col items-end gap-1">
      <Input
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        value={draft}
        disabled={disabled}
        aria-label={label}
        aria-invalid={!valid}
        className="h-7 w-16 bg-background text-right text-xs text-foreground tabular-nums"
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault()
            commit()
          }
          if (event.key === 'Escape') setDraft(String(value))
        }}
      />
      {!valid ? <FieldError className="max-w-40 text-right text-[9px]">{max === undefined ? `≥ ${min}` : `${min}–${max}`}</FieldError> : null}
    </div>
  )
}

export function TuningLinkButton({ label, onClick }: { label: string; onClick?: () => void }) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="xs"
      aria-label={label}
      title={label}
      disabled={!onClick}
      className="director-control-section__action-button"
      onClick={onClick}
    >
      <Settings2 className="director-control-section__action-icon hidden size-3.5" aria-hidden="true" />
      <span className="director-control-section__action-label">{label}</span>
    </Button>
  )
}
