import { useHotkeys } from 'react-hotkeys-hook'
import { isEditableTarget, isNativeTextEditingShortcut } from '@/lib/keyboard'

interface WorkspaceHotkeysOptions {
  onSave?: () => void | Promise<void>
  onOpenCommand?: () => void
  onOpenSearch?: () => void
  onGenerate?: () => void
  onOpenDiff?: () => void
  onToggleRightPanel?: () => void
}

/** 注册工作台全局快捷键，hook 只分发事件，不直接调用业务 API。 */
export function useWorkspaceHotkeys({
  onSave,
  onOpenCommand,
  onOpenSearch,
  onGenerate,
  onOpenDiff,
  onToggleRightPanel,
}: WorkspaceHotkeysOptions) {
  const shouldSkipWorkspaceAction = (event: KeyboardEvent) => {
    return isEditableTarget(event.target) || isNativeTextEditingShortcut(event)
  }

  useHotkeys('meta+s, ctrl+s', (event) => {
    event.preventDefault()
    void onSave?.()
  }, { enableOnFormTags: true, enableOnContentEditable: true }, [onSave])

  useHotkeys('meta+k, ctrl+k', (event) => {
    if (shouldSkipWorkspaceAction(event)) return
    event.preventDefault()
    onOpenCommand?.()
  }, { enableOnFormTags: true, enableOnContentEditable: true }, [onOpenCommand])

  useHotkeys('meta+shift+f, ctrl+shift+f', (event) => {
    const inEditor = event.target instanceof HTMLElement && Boolean(event.target.closest('.editor-content'))
    if (isNativeTextEditingShortcut(event) || (isEditableTarget(event.target) && !inEditor)) return
    event.preventDefault()
    onOpenSearch?.()
  }, { enableOnFormTags: true, enableOnContentEditable: true }, [onOpenSearch])

  useHotkeys('meta+enter, ctrl+enter', (event) => {
    if (shouldSkipWorkspaceAction(event)) return
    event.preventDefault()
    onGenerate?.()
  }, { enableOnFormTags: true, enableOnContentEditable: true }, [onGenerate])

  useHotkeys('meta+shift+d, ctrl+shift+d', (event) => {
    if (shouldSkipWorkspaceAction(event)) return
    event.preventDefault()
    onOpenDiff?.()
  }, { enableOnFormTags: true, enableOnContentEditable: true }, [onOpenDiff])

  useHotkeys('meta+alt+b, ctrl+alt+b', (event) => {
    event.preventDefault()
    onToggleRightPanel?.()
  }, { enableOnFormTags: true, enableOnContentEditable: true }, [onToggleRightPanel])
}
