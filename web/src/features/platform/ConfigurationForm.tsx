import Form from '@rjsf/shadcn'
import { customizeValidator } from '@rjsf/validator-ajv8'
import Ajv2020 from 'ajv/dist/2020'
import { rangeSpec, TranslatableString, type FieldTemplateProps, type WidgetProps } from '@rjsf/utils'
import { useTranslation } from 'react-i18next'
import { useId } from 'react'
import { Field, FieldGroup, FieldContent, FieldLabel, FieldDescription, FieldError } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { Slider } from '@/components/ui/slider'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { ConfigurationDocument } from './api'

const validator = customizeValidator({ AjvClass: Ajv2020, ajvOptionsOverrides: { strict: false } })
const translationKeys = new Map(Object.entries(TranslatableString).map(([key, value]) => [value, key]))

function ConfigurationField({ id, children, displayLabel, label, required, rawDescription, rawErrors, hidden, disabled, schema, uiSchema }: FieldTemplateProps) {
  if (hidden) return <div hidden>{children}</div>
  const scalar = schema.type !== 'object' && schema.type !== 'array' && uiSchema?.['ui:widget'] !== 'textarea'
  if (scalar) return <Field orientation={schema.type === 'boolean' ? 'horizontal' : 'responsive'} data-invalid={Boolean(rawErrors?.length)} data-disabled={disabled} className="min-w-0 gap-3 border-t py-2.5">
    <FieldContent className="min-w-0">
      <FieldLabel htmlFor={id} className="[overflow-wrap:anywhere]">{label}{required && ' *'}</FieldLabel>
      {rawDescription && <FieldDescription className="text-xs leading-relaxed [overflow-wrap:anywhere]">{rawDescription}</FieldDescription>}
      {rawErrors?.length ? <FieldError>{rawErrors.join(' · ')}</FieldError> : null}
    </FieldContent>
    <div className={schema.type === 'boolean' ? 'flex shrink-0 items-center' : 'min-w-0 @md/field-group:basis-64 @md/field-group:shrink-0 @md/field-group:max-w-[50%]'}>{children}</div>
  </Field>
  return <Field data-invalid={Boolean(rawErrors?.length)} data-disabled={disabled} className="min-w-0">
    {displayLabel && <FieldLabel htmlFor={id} className="break-words">{label}{required && ' *'}</FieldLabel>}
    {children}
    {rawDescription && <FieldDescription className="break-words">{rawDescription}</FieldDescription>}
    {rawErrors?.length ? <FieldError>{rawErrors.join(' · ')}</FieldError> : null}
  </Field>
}

function BooleanSetting({ id, value, disabled, readonly, onChange, rawErrors }: WidgetProps) {
  return <Switch id={id} checked={Boolean(value)} disabled={disabled || readonly} onCheckedChange={onChange} aria-invalid={Boolean(rawErrors?.length)} />
}

function RangeSetting({ id, label, value, schema, options, disabled, readonly, onChange }: WidgetProps) {
  const suffix = typeof options.suffix === 'string' ? options.suffix : ''
  return <div className="flex min-w-0 items-center gap-3 py-2">
    <Slider id={id} aria-label={label} aria-valuetext={`${value}${suffix}`} {...rangeSpec(schema)} value={[value ?? schema.minimum ?? 0]}
      disabled={disabled || readonly} onValueChange={([next]) => onChange(next)} />
    <output className="min-w-9 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{value}{suffix}</output>
  </div>
}

function ChoiceSetting({ id, value, options, disabled, readonly, onChange, rawErrors, multiple }: WidgetProps) {
  const { t } = useTranslation()
  const choices = options.enumOptions ?? []
  if (multiple) return <select id={id} multiple value={(value ?? []).map((item: unknown) => String(choices.findIndex(option => option.value === item)))} disabled={disabled || readonly}
    aria-invalid={Boolean(rawErrors?.length)} onChange={event => onChange(Array.from(event.target.selectedOptions, option => choices[Number(option.value)].value))}>
    {choices.map((option, index) => <option key={index} value={String(index)}>{option.label}</option>)}
  </select>
  const selected = choices.findIndex(option => option.value === value)
  return <Select value={selected < 0 ? '' : String(selected)} disabled={disabled || readonly} onValueChange={next => onChange(choices[Number(next)].value)}>
    <SelectTrigger id={id} aria-invalid={Boolean(rawErrors?.length)} className="w-full"><SelectValue placeholder={t('platform.settings.choose')} /></SelectTrigger>
    <SelectContent><SelectGroup>{choices.map((option, index) => <SelectItem key={index} value={String(index)}>{option.label}</SelectItem>)}</SelectGroup></SelectContent>
  </Select>
}

/** The installed package supplies declarative data; all controls and persistence
 * remain host-owned. The same renderer can serve game setup and preview forms. */
export function ConfigurationForm({ definition, values, disabled, onChange }: {
  definition: NonNullable<ConfigurationDocument['form']>
  values: Record<string, unknown>
  disabled: boolean
  onChange: (values: Record<string, unknown>) => void
}) {
  const { t } = useTranslation()
  const idPrefix = useId()
  return <FieldGroup className="gap-0"><Form schema={definition.schema} uiSchema={definition.uiSchema} formData={values} validator={validator}
    idPrefix={idPrefix}
    tagName="div" disabled={disabled} showErrorList={false} liveValidate="onChange" omitExtraData={false}
    templates={{ FieldTemplate: ConfigurationField }} widgets={{ CheckboxWidget: BooleanSetting, SelectWidget: ChoiceSetting, RangeWidget: RangeSetting }}
    translateString={(text, params = []) => params.reduce((result, parameter, index) => result.replaceAll(`%${index + 1}`, parameter), t('platform.form.' + translationKeys.get(text)))}
    transformErrors={errors => errors.map(error => ({ ...error, message: t('platform.settings.invalidValue') }))}
    onChange={event => onChange(event.formData ?? {})}>
    <span />
  </Form></FieldGroup>
}

/** Store only deviations from defaults; removing a table override inherits it. */
export function configurationOverrides(values: Record<string, unknown>, defaults: Record<string, unknown>): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(values)) {
    if (value === undefined) continue
    const fallback = defaults[key]
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const table = fallback && typeof fallback === 'object' && !Array.isArray(fallback) ? fallback as Record<string, unknown> : undefined
      const nested = configurationOverrides(value as Record<string, unknown>, table ?? {})
      if (Object.keys(nested).length || !table) result[key] = nested
    } else if (JSON.stringify(value) !== JSON.stringify(fallback)) result[key] = value
  }
  return result
}
