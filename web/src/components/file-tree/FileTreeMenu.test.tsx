import { render, screen } from '@testing-library/react'
import { FileTreeMenu, FileTreeMenuItem } from './FileTreeMenu'

const anchorRect = {
  top: 100,
  right: 120,
  bottom: 100,
  left: 120,
  width: 0,
  height: 0,
  x: 120,
  y: 100,
}

describe('FileTreeMenu', () => {
  it('renders outside the file-tree host so opening it cannot affect pane sizing', () => {
    render(
      <div data-testid="tree-host">
        <FileTreeMenu anchorRect={anchorRect}>
          <FileTreeMenuItem>Rename</FileTreeMenuItem>
        </FileTreeMenu>
      </div>,
    )

    const menu = screen.getByRole('menu')
    expect(screen.getByTestId('tree-host')).not.toContainElement(menu)
    expect(menu.parentElement).toBe(document.body)
  })

  it('flips toward the viewport when the preferred side would overflow', () => {
    const originalRect = HTMLElement.prototype.getBoundingClientRect
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function getBoundingClientRect(this: HTMLElement) {
      if (this.classList.contains('nova-file-tree-menu')) {
        return { width: 180, height: 200 } as DOMRect
      }
      return originalRect.call(this)
    })

    render(
      <FileTreeMenu anchorRect={{ ...anchorRect, top: 760, bottom: 760, left: 1000, right: 1000, x: 1000, y: 760 }}>
        <FileTreeMenuItem>Rename</FileTreeMenuItem>
      </FileTreeMenu>,
    )

    expect(screen.getByRole('menu')).toHaveStyle({ left: '816px', top: '556px', visibility: 'visible' })
    rectSpy.mockRestore()
  })
})
