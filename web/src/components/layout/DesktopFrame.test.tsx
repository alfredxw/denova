import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { DesktopFrame } from './DesktopFrame'
import type { DesktopBridge } from '@/lib/desktop'

afterEach(() => { delete window.denovaDesktop })

it('leaves browser and macOS workspaces without Windows chrome', () => {
  const { rerender } = render(<DesktopFrame><input aria-label="Editor" /></DesktopFrame>)
  expect(screen.queryByRole('navigation')).toBeNull()
  window.denovaDesktop = { platform: 'darwin' } as DesktopBridge
  rerender(<DesktopFrame><input aria-label="Editor" /></DesktopFrame>)
  expect(screen.queryByRole('navigation')).toBeNull()
  expect(screen.getByRole('textbox')).toBeVisible()
})

it('opens the native menu at its trigger and preserves pointer editing focus', async () => {
  let closeMenu!: () => void
  const openMenu = vi.fn(() => new Promise<void>(resolve => { closeMenu = resolve }))
  window.denovaDesktop = { platform: 'win32', openMenu } as unknown as DesktopBridge
  render(<DesktopFrame><input aria-label="Editor" defaultValue="Selected text" /></DesktopFrame>)
  const editor = screen.getByRole('textbox') as HTMLInputElement
  editor.focus()
  editor.setSelectionRange(0, 8)
  const menu = screen.getAllByRole('button')[0]
  vi.spyOn(menu, 'getBoundingClientRect').mockReturnValue({ left: 12, bottom: 36 } as DOMRect)
  expect(fireEvent.mouseDown(menu)).toBe(false)
  fireEvent.click(menu)
  expect(document.activeElement).toBe(editor)
  expect([editor.selectionStart, editor.selectionEnd]).toEqual([0, 8])
  expect(openMenu).toHaveBeenCalledWith('application', { x: 12, y: 36 })
  expect(menu).toHaveAttribute('aria-expanded', 'true')
  closeMenu()
  await waitFor(() => expect(menu).toHaveAttribute('aria-expanded', 'false'))
})
