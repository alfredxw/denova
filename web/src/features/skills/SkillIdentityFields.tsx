import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/forms/form-field'
import { FieldLegend, FieldSet } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import type { SkillScope, SkillScopeInfo } from '@/lib/api'
import { PreviewRow } from './skill-form-fields'
import { scopeLabel } from './skill-utils'

interface SkillIdentityFieldsProps {
  scopes: SkillScopeInfo[]
  scope: SkillScope
  onScopeChange: (scope: SkillScope) => void
  name: string
  onNameChange: (name: string) => void
  description: string
  onDescriptionChange: (description: string) => void
  invalidName: boolean
  descriptionRequired?: boolean
  targetName: string
  targetPath: string
  showPreview?: boolean
}

/** Shared scope/name/description fields for both Skill creation and configuration. */
export function SkillIdentityFields({
  scopes,
  scope,
  onScopeChange,
  name,
  onNameChange,
  description,
  onDescriptionChange,
  invalidName,
  descriptionRequired = false,
  targetName,
  targetPath,
  showPreview = false,
}: SkillIdentityFieldsProps) {
  const { t } = useTranslation()
  const nameId = useId()
  const descriptionId = useId()
  const trimmedDescription = description.trim()

  return (
    <>
      <div className="grid gap-3 md:grid-cols-2">
        <FieldSet className="min-w-0 gap-1.5 text-xs">
          <FieldLegend variant="label" className="mb-0 text-[11px] font-normal text-muted-foreground">
            {t('skills.create.scope')}
          </FieldLegend>
          <ToggleGroup
            type="single"
            value={scope}
            onValueChange={(nextScope) => {
              if (nextScope) onScopeChange(nextScope as SkillScope)
            }}
            variant="outline"
            size="sm"
            spacing={1}
            aria-label={t('skills.create.scope')}
            className="w-full"
          >
            {scopes.map((item) => (
              <ToggleGroupItem
                key={item.scope}
                value={item.scope}
                className={`nova-nav-item h-8 min-w-0 flex-1 rounded-[var(--nova-radius)] ${scope === item.scope ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
              >
                {scopeLabel(item.scope, t)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </FieldSet>
        <FormField
          htmlFor={nameId}
          label={t('skills.create.name')}
          description={invalidName ? undefined : t('skills.create.nameHint')}
          error={invalidName ? t('skills.create.invalidName') : undefined}
        >
          <Input
            id={nameId}
            value={name}
            onChange={(event) => onNameChange(event.target.value)}
            aria-invalid={invalidName}
            placeholder={t('skills.create.namePlaceholder')}
            className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 font-mono outline-none aria-invalid:border-[var(--nova-danger)]"
          />
        </FormField>
      </div>

      {showPreview ? (
        <div className="grid gap-2 md:grid-cols-2">
          <PreviewRow label={t('skills.create.preview.command')} value={`/${targetName}`} />
          <PreviewRow label={t('skills.create.preview.scope')} value={scopeLabel(scope, t)} />
          <PreviewRow label={t('skills.create.preview.path')} value={targetPath || t('skills.agent.pathFallback')} wide />
        </div>
      ) : null}

      <FormField
        htmlFor={descriptionId}
        label={t('skills.create.description')}
        description={!descriptionRequired || trimmedDescription ? t('skills.create.descriptionHint') : undefined}
        error={descriptionRequired && !trimmedDescription ? t('skills.config.descriptionRequired') : undefined}
      >
        <Input
          id={descriptionId}
          value={description}
          onChange={(event) => onDescriptionChange(event.target.value)}
          aria-invalid={descriptionRequired && !trimmedDescription}
          placeholder={t('skills.create.descriptionPlaceholder')}
          className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 outline-none"
        />
      </FormField>
    </>
  )
}
