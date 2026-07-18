import { SlidersHorizontal, Sparkles } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import { BUILTIN_WRITING_SKILLS, DEFAULT_WRITING_SKILL, type WritingSkillOption } from '@/hooks/useWritingSkillOptions'
import { PersistedSettingsMenuSub } from './PersistedSettingsMenuSub'

interface WritingComposerSettingsMenuProps {
  enabled: boolean
  tellers: Teller[]
  tellerID: string
  imagePresets: ImagePreset[]
  imagePresetID: string
  writingSkills: WritingSkillOption[]
  writingSkill: string
  savingTeller?: boolean
  savingImagePreset?: boolean
  savingWritingSkill?: boolean
  onTellerChange: (value: string) => void | Promise<unknown>
  onImagePresetChange: (value: string) => void | Promise<unknown>
  onWritingSkillChange: (value: string) => void | Promise<unknown>
}

/** Writing-specific menu composed from the generic persisted-setting submenu. */
export function WritingComposerSettingsMenu({
  enabled,
  tellers,
  tellerID,
  imagePresets,
  imagePresetID,
  writingSkills,
  writingSkill,
  savingTeller,
  savingImagePreset,
  savingWritingSkill,
  onTellerChange,
  onImagePresetChange,
  onWritingSkillChange,
}: WritingComposerSettingsMenuProps) {
  const { t } = useTranslation()
  const selectedTeller = tellers.find((item) => item.id === tellerID) ?? tellers.find((item) => item.id === 'classic') ?? tellers[0]
  const normalizedPresets = useMemo(() => (
    imagePresets.some((preset) => preset.id === imagePresetID)
      ? imagePresets
      : [{ id: imagePresetID || 'game-cg', name: imagePresetID || 'game-cg', description: '', prompt: '', custom: true, version: 1 }, ...imagePresets]
  ), [imagePresetID, imagePresets])
  const selectedPreset = normalizedPresets.find((item) => item.id === imagePresetID) ?? normalizedPresets.find((item) => item.id === 'game-cg') ?? normalizedPresets[0]
  const normalizedSkills = useMemo(() => (
    writingSkills.some((option) => option.name === writingSkill)
      ? writingSkills
      : [fallbackWritingSkillOption(writingSkill || DEFAULT_WRITING_SKILL), ...writingSkills]
  ), [writingSkill, writingSkills])
  const selectedSkill = normalizedSkills.find((option) => option.name === writingSkill) ?? normalizedSkills.find((option) => option.name === DEFAULT_WRITING_SKILL)
  const skillLabel = selectedSkill
    ? `${writingSkillLabel(selectedSkill.name, t)} · ${t(`chat.writingSkill.source.${selectedSkill.scope}`)}`
    : writingSkill

  return (
    <>
      {tellers.length > 0 ? (
        <PersistedSettingsMenuSub
          icon={SlidersHorizontal}
          label={t('chat.teller')}
          title={t('chat.tellerTitle')}
          currentLabel={selectedTeller?.name || tellerID}
          value={selectedTeller?.id || tellerID}
          options={tellers.map((item) => ({ id: item.id, label: item.name }))}
          saving={savingTeller}
          disabled={!enabled}
          onValueChange={onTellerChange}
        />
      ) : null}
      <PersistedSettingsMenuSub
        icon={Sparkles}
        label={t('chat.imagePreset')}
        title={t('chat.imagePresetTitle')}
        currentLabel={selectedPreset?.name || imagePresetID}
        value={selectedPreset?.id || imagePresetID}
        options={normalizedPresets.map((item) => ({ id: item.id, label: item.name || item.id }))}
        saving={savingImagePreset}
        disabled={!enabled}
        onValueChange={onImagePresetChange}
      />
      <PersistedSettingsMenuSub
        icon={Sparkles}
        label={t('chat.writingSkill')}
        title={selectedSkill?.path || t('chat.writingSkillTitle')}
        currentLabel={skillLabel}
        value={selectedSkill?.name || writingSkill}
        options={normalizedSkills.map((option) => ({
          id: option.name,
          label: writingSkillLabel(option.name, t),
          meta: t(`chat.writingSkill.source.${option.scope}`),
        }))}
        saving={savingWritingSkill}
        disabled={!enabled}
        emptyLabel={t('chat.writingSkill.empty')}
        onValueChange={onWritingSkillChange}
      />
    </>
  )
}

function fallbackWritingSkillOption(name: string): WritingSkillOption {
  const scope = BUILTIN_WRITING_SKILLS.includes(name as typeof BUILTIN_WRITING_SKILLS[number]) ? 'builtin' : 'workspace'
  return { name, description: '', scope, path: '', active: true, agent: 'ide' }
}

function writingSkillLabel(name: string, t: ReturnType<typeof useTranslation>['t']): string {
  switch (name) {
    case 'novel-lite':
      return t('chat.writingSkill.preset.lite')
    case 'novel-standard':
      return t('chat.writingSkill.preset.standard')
    case 'novel-heavy':
      return t('chat.writingSkill.preset.heavy')
    default:
      return name
  }
}
