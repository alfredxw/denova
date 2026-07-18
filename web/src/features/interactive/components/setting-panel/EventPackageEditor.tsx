import { useTranslation } from 'react-i18next'
import type { EventPackageModule } from '../../types'
import { PresetConfigSectionEditor } from '../preset-config/PresetConfigSectionEditor'
import { PresetEmptyState } from '../preset-config/PresetEmptyState'
import { PresetModuleEditorShell } from '../preset-config/PresetModuleEditorShell'
import { EventPackageVisualEditor } from '../preset-config/visual-editors'
import { eventPackageSummaryCount } from './editor-shared'

export function EventPackageEditor({
  draft,
  setDraft,
  onSave,
  onValidityChange,
}: {
  draft: EventPackageModule | null
  setDraft: (draft: EventPackageModule | null) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()

  if (!draft) {
    return <PresetEmptyState title={t('settingPanel.editor.noEventPackageSelected')} description={t('settingPanel.editor.noEventPackageSelectedDesc')} />
  }

  return (
    <PresetModuleEditorShell draft={draft} setDraft={setDraft}>
      <PresetConfigSectionEditor
        sectionId="event-package.events"
        resetKey={`${draft.id}:events`}
        title={t('settingPanel.presetConfig.eventCards')}
        description={t('settingPanel.editor.eventPackageEventsDesc')}
        value={draft}
        summary={t('settingPanel.eventPackage.summaryCount', { count: eventPackageSummaryCount(draft) })}
        onChange={setDraft}
        onSave={onSave}
        onValidityChange={onValidityChange}
      >
        {(props) => <EventPackageVisualEditor {...props} />}
      </PresetConfigSectionEditor>
    </PresetModuleEditorShell>
  )
}
