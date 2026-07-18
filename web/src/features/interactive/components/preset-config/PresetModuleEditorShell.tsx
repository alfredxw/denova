import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PresetMetadataPanel } from './PresetEditorChrome'
import { presetStatusLabel } from './preset-status'

interface PresetModuleEditorShellProps<T> {
  draft: T
  setDraft: (draft: T | null) => void
  contentClassName?: string
  children: ReactNode
}

/** Metadata plus content shell shared by event, rule and actor-state modules. */
export function PresetModuleEditorShell<T extends { name: string; description: string; custom: boolean; builtin_overridden?: boolean }>({
  draft,
  setDraft,
  contentClassName = 'grid min-h-0 flex-1 gap-4 overflow-y-auto p-3 sm:p-4',
  children,
}: PresetModuleEditorShellProps<T>) {
  const { t } = useTranslation()
  const editHint = draft.custom ? t('settingPanel.storyDirector.customEditable') : t('settingPanel.storyDirector.builtInCopyHint')

  return (
    <div className="preset-module-editor flex min-h-0 flex-1 flex-col overflow-hidden">
      <PresetMetadataPanel
        name={draft.name}
        description={draft.description}
        status={presetStatusLabel(draft, t)}
        hint={editHint}
        onNameChange={(name) => setDraft({ ...draft, name })}
        onDescriptionChange={(description) => setDraft({ ...draft, description })}
      />
      <div className={contentClassName}>{children}</div>
    </div>
  )
}
