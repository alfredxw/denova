import { useEffect, useState, type ReactNode } from 'react'
import { ChevronDown, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Field, FieldContent, FieldDescription, FieldError, FieldTitle } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'

export type TuningSource = 'preset' | 'story' | 'off' | 'locked'

export function TuningSection({
  icon,
  title,
  summary,
  open,
  onOpenChange,
  children,
}: {
  icon: ReactNode
  title: string
  summary: string
  open: boolean
  onOpenChange: (open: boolean) => void
  children: ReactNode
}) {
  return (
    <Collapsible open={open} onOpenChange={onOpenChange} className="overflow-hidden rounded-xl border border-border bg-card">
      <CollapsibleTrigger asChild>
        <button type="button" className="flex w-full min-w-0 items-center gap-3 px-3 py-3 text-left transition-colors hover:bg-muted/50">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-background text-[var(--director-brass)]">{icon}</span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-xs font-semibold text-foreground">{title}</span>
            <span className="mt-0.5 block truncate text-[10px] text-muted-foreground">{summary}</span>
          </span>
          <ChevronDown className={cn('size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="flex flex-col gap-3 border-t border-border px-3 py-3">{children}</div>
      </CollapsibleContent>
    </Collapsible>
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
    <Field orientation="responsive" data-disabled={disabled || undefined} className="min-w-0 rounded-lg border border-border bg-background/50 p-2.5">
      <FieldContent className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
          <FieldTitle className="text-xs">{title}</FieldTitle>
          {source ? <TuningSourceBadge source={source} /> : null}
          {busy ? <Loader2 className="size-3 animate-spin text-muted-foreground" aria-label={title} /> : null}
        </div>
        {description ? <FieldDescription className="text-[10px] leading-4">{description}</FieldDescription> : null}
      </FieldContent>
      <div className="flex min-w-0 shrink-0 items-center justify-end">{children}</div>
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
      <SelectTrigger size="sm" aria-label={label} className="w-36 min-w-0 max-w-full bg-background text-xs text-foreground">
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
        className="h-7 w-20 bg-background text-right text-xs text-foreground tabular-nums"
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

export function TuningLinkButton({ children, onClick }: { children: ReactNode; onClick?: () => void }) {
  return <Button type="button" variant="ghost" size="xs" disabled={!onClick} onClick={onClick}>{children}</Button>
}
