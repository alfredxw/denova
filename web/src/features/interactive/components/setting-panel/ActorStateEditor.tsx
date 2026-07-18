import { Dice5 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { ActorStateModule, RuleSystemModule } from '../../types'
import { ActorStateExplorer, type ExplorerProps } from '../preset-config/actor-state-explorer'
import { PresetConfigSectionEditor } from '../preset-config/PresetConfigSectionEditor'
import { PresetEmptyState } from '../preset-config/PresetEditorChrome'
import { ModuleEditorShell, usePresetSectionValidity } from './editor-shared'

export function ActorStateEditor({
  draft,
  ruleSystems = [],
  setDraft,
  onOpenRuleSystem,
  onSave,
  onValidityChange,
}: {
  draft: ActorStateModule | null
  ruleSystems?: RuleSystemModule[]
  setDraft: (draft: ActorStateModule | null) => void
  onOpenRuleSystem?: (id: string) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const setSectionValid = usePresetSectionValidity(draft?.id || '', onValidityChange)

  if (!draft) {
    return <PresetEmptyState title={t('settingPanel.editor.noActorStateSelected')} description={t('settingPanel.editor.noActorStateSelectedDesc')} />
  }

  const explorerValue: ExplorerProps['value'] = draft.actor_state || {
    templates: [],
    trait_pools: [],
    initial_actors: [],
  }

  const handleExplorerChange = (value: ExplorerProps['value']) => {
    setDraft({
      ...draft,
      actor_state: value,
    })
  }
  const linkedRuleSystems = ruleSystems.filter((rule) => rule.actor_state_id === draft.id)

  return (
    <ModuleEditorShell
      draft={draft}
      setDraft={setDraft}
      metadata="compact"
      contentClassName="flex min-h-[320px] flex-1 p-0"
    >
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <div className="flex min-h-10 shrink-0 items-center gap-2 overflow-x-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-1.5">
          <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">
            {linkedRuleSystems.length
              ? t('settingPanel.actorState.usedByChecks', { count: linkedRuleSystems.length })
              : t('settingPanel.actorState.notUsedByChecks')}
          </span>
          {linkedRuleSystems.map((rule) => (
            <Button
              key={rule.id}
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 shrink-0 rounded-full px-2.5 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              onClick={() => onOpenRuleSystem?.(rule.id)}
              aria-label={t('settingPanel.actorState.openCheck', { name: rule.name || rule.id })}
            >
              <Dice5 data-icon="inline-start" />
              {rule.name || rule.id}
            </Button>
          ))}
        </div>
        <PresetConfigSectionEditor
          sectionId="actor-state.unified"
          resetKey={`${draft.id}:unified`}
          title={t('settingPanel.actorState.title')}
          description={t('settingPanel.actorState.description')}
          value={explorerValue}
          summary={t('settingPanel.actorState.summaryCount', {
            templates: explorerValue.templates?.length || 0,
            pools: explorerValue.trait_pools?.length || 0,
            actors: explorerValue.initial_actors?.length || 0,
          })}
          onChange={handleExplorerChange}
          onSave={onSave}
          onValidityChange={(valid) => setSectionValid('actor_state', valid)}
          layout="flush"
        >
          {(props) => (
            <ActorStateExplorer
              value={props.value}
              onChange={props.onChange}
              onValidityChange={props.onValidityChange}
              layout="attached"
            />
          )}
        </PresetConfigSectionEditor>
      </div>
    </ModuleEditorShell>
  )
}
