import { useEffect, useId, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldGroup, FieldLabel, FieldLegend, FieldSet } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

const REPLY_LENGTH_PRESETS = [600, 800, 1200, 1600, 2000, 2400] as const

interface ReplyLengthSettingProps {
  value: number
  label: string
  disabled?: boolean
  onCommit: (value: number) => void
}

export function ReplyLengthSetting({
  value,
  label,
  disabled,
  onCommit,
}: ReplyLengthSettingProps) {
  const { t } = useTranslation()
  const customInputID = useId()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(String(value))
  const parsed = Number(draft)
  const valid = Number.isInteger(parsed) && parsed >= 1
  const selectedPreset = REPLY_LENGTH_PRESETS.some((preset) => preset === value) ? String(value) : ''

  useEffect(() => setDraft(String(value)), [value])

  function selectPreset(nextValue: string) {
    if (!nextValue) {
      setOpen(false)
      return
    }
    const next = Number(nextValue)
    setDraft(nextValue)
    setOpen(false)
    if (next !== value) onCommit(next)
  }

  function commitCustom() {
    if (!valid) return
    setDraft(String(parsed))
    setOpen(false)
    if (parsed !== value) onCommit(parsed)
  }

  return (
    <Popover
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (nextOpen) setDraft(String(value))
      }}
    >
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled}
          aria-label={label}
          className="w-20 justify-between bg-background text-xs font-normal text-foreground tabular-nums"
        >
          <span>{value}</span>
          <ChevronDown data-icon="inline-end" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-64">
        <form
          onSubmit={(event) => {
            event.preventDefault()
            commitCustom()
          }}
        >
          <FieldGroup className="gap-3">
            <FieldSet className="gap-1.5">
              <FieldLegend variant="label" className="mb-0 text-xs">
                {t('directorPanel.tuning.agent.replyLengthPresets')}
              </FieldLegend>
              <ToggleGroup
                type="single"
                value={selectedPreset}
                variant="outline"
                size="sm"
                spacing={1}
                aria-label={t('directorPanel.tuning.agent.replyLengthPresets')}
                className="grid w-full grid-cols-3"
                onValueChange={selectPreset}
              >
                {REPLY_LENGTH_PRESETS.map((preset) => (
                  <ToggleGroupItem key={preset} value={String(preset)} className="min-w-0 px-2 tabular-nums">
                    {preset}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </FieldSet>
            <Field data-invalid={valid ? undefined : true} className="gap-1.5">
              <FieldLabel htmlFor={customInputID} className="text-xs">
                {t('directorPanel.tuning.agent.replyLengthCustom')}
              </FieldLabel>
              <div className="flex items-start gap-1.5">
                <Input
                  id={customInputID}
                  type="number"
                  inputMode="numeric"
                  min={1}
                  value={draft}
                  aria-invalid={!valid}
                  className="h-7 bg-background text-right text-xs text-foreground tabular-nums"
                  onChange={(event) => setDraft(event.target.value)}
                />
                <Button type="submit" size="sm" disabled={!valid}>
                  {t('common.save')}
                </Button>
              </div>
              {!valid ? <FieldError className="text-[10px]">≥ 1</FieldError> : null}
            </Field>
          </FieldGroup>
        </form>
      </PopoverContent>
    </Popover>
  )
}
