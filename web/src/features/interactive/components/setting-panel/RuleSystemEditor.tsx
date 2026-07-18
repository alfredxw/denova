import { useTranslation } from 'react-i18next'
import type { ActorStateModule, RuleSystemModule } from '../../types'
import { PresetConfigSectionEditor } from '../preset-config/PresetConfigSectionEditor'
import { PresetEmptyState } from '../preset-config/PresetEditorChrome'
import { normalizeTRPGSystem } from '../preset-config/ruleTemplates'
import { TRPGSystemVisualEditor } from '../preset-config/TRPGSystemVisualEditor'
import { ModuleEditorShell, usePresetSectionValidity } from './editor-shared'

export function RuleSystemEditor({
  draft,
  actorStates = [],
  setDraft,
  onOpenActorState,
  onSave,
  onValidityChange,
}: {
  draft: RuleSystemModule | null
  actorStates?: ActorStateModule[]
  setDraft: (draft: RuleSystemModule | null) => void
  onOpenActorState?: (id: string) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const setSectionValid = usePresetSectionValidity(draft?.id || '', onValidityChange)

  if (!draft) {
    return <PresetEmptyState title={t('settingPanel.editor.noRuleSystemSelected')} description={t('settingPanel.editor.noRuleSystemSelectedDesc')} />
  }

  return (
    <ModuleEditorShell draft={draft} setDraft={setDraft} metadata="compact">
      <PresetConfigSectionEditor
        sectionId="rule-system.trpg-system"
        resetKey={`${draft.id}:trpg_system`}
        title={t('settingPanel.storyDirector.trpgSystem')}
        description={t('settingPanel.storyDirector.trpgSystemDesc')}
        value={normalizeTRPGSystem(draft.trpg_system || { rule_templates: [] })}
        summary={t('settingPanel.storyDirector.trpgSystemSummary', { count: draft.trpg_system?.rule_templates?.length || 0 })}
        onChange={(trpg_system) => setDraft({ ...draft, trpg_system: normalizeTRPGSystem(trpg_system) })}
        onSave={onSave}
        onValidityChange={(valid) => setSectionValid('trpg_system', valid)}
      >
        {(props) => (
          <TRPGSystemVisualEditor
            {...props}
            actorStateId={draft.actor_state_id}
            actorStates={actorStates}
            onActorStateChange={(actor_state_id) => setDraft({ ...draft, actor_state_id })}
            onOpenActorState={onOpenActorState}
          />
        )}
      </PresetConfigSectionEditor>
    </ModuleEditorShell>
  )
}
