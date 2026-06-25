const EDITABLE_FORM_TAGS = new Set(['INPUT', 'TEXTAREA', 'SELECT'])

interface KeyboardShortcutEvent {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
}

interface ComposingKeyboardEvent {
  isComposing?: boolean
  keyCode?: number
  nativeEvent?: {
    isComposing?: boolean
    keyCode?: number
  }
}

interface PropagatingKeyboardShortcutEvent extends KeyboardShortcutEvent {
  stopPropagation: () => void
}

/** 判断快捷键事件是否来自用户正在编辑文本的区域。 */
export function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  let node: HTMLElement | null = target
  while (node) {
    if (node.isContentEditable || node.contentEditable === 'true') return true
    node = node.parentElement
  }
  if (EDITABLE_FORM_TAGS.has(target.tagName)) return true
  return Boolean(target.closest('[contenteditable="true"]'))
}

/** 判断是否为需要保留给浏览器/编辑组件原生文本行为的快捷键。 */
export function isNativeTextEditingShortcut(event: KeyboardShortcutEvent): boolean {
  if (event.altKey || (!event.metaKey && !event.ctrlKey)) return false
  const key = event.key.toLowerCase()
  return ['a', 'c', 'x', 'v', 'z', 'y'].includes(key)
}

/** 让输入控件保留原生文本快捷键，同时不冒泡到工作台级快捷键。 */
export function preserveNativeTextEditingShortcut(event: PropagatingKeyboardShortcutEvent): void {
  if (isNativeTextEditingShortcut(event)) {
    event.stopPropagation()
  }
}

/** 判断当前事件是否为应用级保存快捷键。 */
export function isSaveShortcut(event: KeyboardShortcutEvent): boolean {
  if (event.altKey || (!event.metaKey && !event.ctrlKey)) return false
  return event.key.toLowerCase() === 's'
}

/** 判断键盘事件是否仍在输入法组合态，避免 Enter 被误当成发送。 */
export function isComposingKeyboardEvent(event: ComposingKeyboardEvent): boolean {
  return Boolean(event.isComposing || event.nativeEvent?.isComposing || event.keyCode === 229 || event.nativeEvent?.keyCode === 229)
}
