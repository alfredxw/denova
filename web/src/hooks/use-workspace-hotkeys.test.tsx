import { renderHook } from '@testing-library/react'
import { useWorkspaceHotkeys } from './use-workspace-hotkeys'

describe('useWorkspaceHotkeys', () => {
  it('leaves bare Escape available to editors and IMEs', () => {
    const onToggleRightPanel = vi.fn()
    renderHook(() => useWorkspaceHotkeys({ onToggleRightPanel }))

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))

    expect(onToggleRightPanel).not.toHaveBeenCalled()
  })

  it('toggles the right sidebar with the VS Code shortcut', () => {
    const onToggleRightPanel = vi.fn()
    renderHook(() => useWorkspaceHotkeys({ onToggleRightPanel }))

    document.dispatchEvent(new KeyboardEvent('keydown', {
      key: 'b',
      code: 'KeyB',
      ctrlKey: true,
      altKey: true,
      bubbles: true,
      cancelable: true,
    }))

    expect(onToggleRightPanel).toHaveBeenCalledOnce()
  })
})
