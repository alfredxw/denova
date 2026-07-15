import { AnimatePresence, motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { novaEase } from '@/features/motion/motion-tokens'
import { parseNumberInput } from '../../utils'
import type { ActorStateField, ActorStateTemplate } from '../../../../types'
import type { ExplorerProps } from '../types'
import { fieldNodeId } from '../build-tree'
import { FieldTypeBadge } from '../shared/FieldTypeBadge'
import { VisibilityBadge } from '../shared/VisibilityBadge'
import { StateValueEditor } from '../shared/StateValueEditor'
import { DetailResponsiveGrid, DetailStack } from './DetailLayout'
import { actorStateFieldNameHasPathSeparator } from '../validation'

const FIELD_TYPES = ['number', 'string', 'bool', 'enum', 'object', 'list'] as const
const VISIBILITY_OPTIONS = ['visible', 'hidden', 'spoiler'] as const

interface FieldDetailEditorProps {
  field: ActorStateField
  fieldIndex: number
  template: ActorStateTemplate
  templateIndex: number
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
  onIdChange: (nextId: string) => void
}

export function FieldDetailEditor({
  field,
  fieldIndex,
  template,
  templateIndex,
  value,
  onChange,
  onIdChange,
}: FieldDetailEditorProps) {
  const { t } = useTranslation()
  const updateField = (patch: Partial<ActorStateField>) => {
    const templates = [...(value.templates || [])]
    const tpl = { ...templates[templateIndex] }
    const fields = [...(tpl.fields || [])]
    const nextField = { ...field, ...patch }
    fields[fieldIndex] = nextField
    tpl.fields = fields
    templates[templateIndex] = tpl
    const nextNodeId = fieldNodeId(template.id, nextField, fieldIndex)
    if (nextNodeId !== fieldNodeId(template.id, field, fieldIndex)) onIdChange(nextNodeId)
    onChange({ ...value, templates })
  }

  const isEnum = field.type === 'enum'
  const isNumber = field.type === 'number'
	const normalizedName = normalizeStateName(field.name)
	const nameConflict = normalizedName !== '' && (template.fields || []).some((candidate, index) => (
		index !== fieldIndex && normalizeStateName(candidate.name) === normalizedName
	))
	const hasPathSeparator = actorStateFieldNameHasPathSeparator(field.name)

  return (
    <DetailStack>
      {/* Field header */}
      <motion.div
        initial={{ opacity: 0, y: 4 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.2, ease: novaEase }}
        className="flex flex-wrap items-center gap-2"
      >
        <FieldTypeBadge type={field.type} />
        {field.visibility ? <VisibilityBadge visibility={field.visibility} /> : null}
        <span className="text-[10px] text-[var(--nova-text-faint)]">{t('settingPanel.actorState.explorer.nameIsId')}</span>
      </motion.div>

      <DetailResponsiveGrid className="items-start">
        {/* Basic properties */}
        <section className="space-y-3">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
              {t('settingPanel.actorState.explorer.fieldInfo')}
            </span>
          </div>

          <div className="grid gap-3">
            <FormField label={t('settingPanel.field.name')}>
              <Input
                className="nova-field h-8 text-xs focus-visible:ring-0"
                value={field.name || ''}
                onChange={(e) => updateField({ name: e.target.value })}
                placeholder={t('settingPanel.actorState.explorer.fieldNamePlaceholder')}
				aria-invalid={hasPathSeparator || nameConflict || undefined}
              />
            </FormField>
          </div>
			{hasPathSeparator ? (
				<div role="alert" className="rounded-[12px] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">
					{t('settingPanel.actorState.explorer.pathSeparatorInvalid')}
				</div>
			) : nameConflict ? (
				<div role="alert" className="rounded-[12px] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">
					{t('settingPanel.actorState.explorer.nameConflict')}
				</div>
			) : (
				<div className="rounded-[12px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-muted)]">
					{t('settingPanel.actorState.explorer.nameIdHelp')}
				</div>
			)}

          <div className="grid gap-3 sm:grid-cols-2">
            <FormField label={t('settingPanel.actorState.explorer.type')}>
              <Select
                value={field.type}
                onValueChange={(v) => updateField({ type: v })}
              >
                <SelectTrigger className="nova-field h-8 text-xs focus:ring-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  <SelectGroup>
                    {FIELD_TYPES.map((fieldType) => (
                      <SelectItem key={fieldType} value={fieldType}>{fieldType}</SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </FormField>
            <FormField label={t('settingPanel.actorState.explorer.visibility')}>
              <Select
                value={field.visibility || 'visible'}
                onValueChange={(v) => updateField({ visibility: v as ActorStateField['visibility'] })}
              >
                <SelectTrigger className="nova-field h-8 text-xs focus:ring-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  <SelectGroup>
                    {VISIBILITY_OPTIONS.map((visibility) => (
                      <SelectItem key={visibility} value={visibility}>
                        {t(`settingPanel.actorState.explorer.${visibility}`)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </FormField>
          </div>

          {/* Number-specific fields */}
          <AnimatePresence>
            {isNumber ? (
              <motion.div
                key="number-fields"
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: 'auto', opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.15, ease: novaEase }}
                className="overflow-hidden"
              >
                <div className="grid gap-3 pt-1 sm:grid-cols-3">
                  <FormField label={t('settingPanel.actorState.explorer.minimum')}>
                    <Input
                      className="nova-field h-8 text-xs focus-visible:ring-0"
                      inputMode="decimal"
                      value={field.min !== undefined ? String(field.min) : ''}
                      onChange={(e) => updateField({ min: parseNumberInput(e.target.value) })}
                    />
                  </FormField>
                  <FormField label={t('settingPanel.actorState.explorer.maximum')}>
                    <Input
                      className="nova-field h-8 text-xs focus-visible:ring-0"
                      inputMode="decimal"
                      value={field.max !== undefined ? String(field.max) : ''}
                      onChange={(e) => updateField({ max: parseNumberInput(e.target.value) })}
                    />
                  </FormField>
                  <FormField label={t('settingPanel.actorState.explorer.order')}>
                    <Input
                      className="nova-field h-8 text-xs focus-visible:ring-0"
                      inputMode="numeric"
                      value={field.order !== undefined ? String(field.order) : ''}
                      onChange={(e) => updateField({ order: parseNumberInput(e.target.value) })}
                    />
                  </FormField>
                </div>
              </motion.div>
            ) : null}
          </AnimatePresence>

          {/* Enum-specific fields */}
          <AnimatePresence>
            {isEnum ? (
              <motion.div
                key="enum-fields"
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: 'auto', opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.15, ease: novaEase }}
                className="overflow-hidden"
              >
                <div className="pt-1">
                  <FormField label={t('settingPanel.actorState.explorer.optionsLabel')}>
                    <Input
                      className="nova-field h-8 text-xs focus-visible:ring-0"
                      value={(field.options || []).join('，')}
                      onChange={(e) => {
                        const opts = e.target.value
                          .split(/[，,]/)
                          .map((s) => s.trim())
                          .filter(Boolean)
                        updateField({ options: opts })
                      }}
                      placeholder={t('settingPanel.actorState.explorer.optionsPlaceholder')}
                    />
                  </FormField>
                </div>
              </motion.div>
            ) : null}
          </AnimatePresence>
        </section>

        <DetailStack>
          {/* Default value */}
          <section className="space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
                {t('settingPanel.actorState.explorer.defaultValue')}
              </span>
            </div>
            <StateValueEditor
              type={field.type}
              value={field.default}
              onChange={(v) => updateField({ default: v })}
              options={field.options}
              min={field.min}
              max={field.max}
            />
          </section>

          {/* Description + instruction */}
          <section className="space-y-3">
            <div className="flex items-center gap-2">
              <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
                {t('settingPanel.actorState.explorer.instructions')}
              </span>
            </div>
            <FormField label={t('common.description')}>
              <Textarea
                className="nova-field min-h-[48px] resize-none text-xs focus-visible:ring-0"
                value={field.description || ''}
                onChange={(e) => updateField({ description: e.target.value })}
                placeholder={t('settingPanel.actorState.explorer.fieldDescriptionPlaceholder')}
              />
            </FormField>
            <FormField label={t('settingPanel.actorState.updateInstruction')}>
              <Textarea
                className="nova-field min-h-[48px] resize-none text-xs focus-visible:ring-0"
                value={field.update_instruction || ''}
                onChange={(e) => updateField({ update_instruction: e.target.value })}
                placeholder={t('settingPanel.actorState.explorer.updateInstructionPlaceholder')}
              />
            </FormField>
          </section>
        </DetailStack>
      </DetailResponsiveGrid>
    </DetailStack>
  )
}

function normalizeStateName(value: string | undefined) {
	return (value || '').normalize('NFKC').trim().toLocaleLowerCase()
}

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <label className="text-[11px] text-[var(--nova-text-faint)]">{label}</label>
      {children}
    </div>
  )
}
