import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useActivityBarWidth } from './use-activity-bar-width'

function Navigation({ enabled = true }: { enabled?: boolean }) {
  const [expanded, setExpanded] = useState(true)
  const sizing = useActivityBarWidth({ enabled, expanded, onToggle: () => setExpanded(value => !value) })
  return (
    <div ref={sizing.wrapperRef} data-testid="wrapper" style={{ '--sidebar-width': `${sizing.width}px` } as React.CSSProperties}>
      <div data-slot="sidebar">
        <div data-slot="sidebar-container" data-testid="navigation" data-expanded={sizing.expanded}>
          <div role="separator" onPointerDown={sizing.onResizePointerDown} onKeyDown={sizing.onResizeKeyDown} />
        </div>
      </div>
      <output>{expanded ? 'expanded' : 'collapsed'}</output>
    </div>
  )
}

function startDrag() {
  const navigation = screen.getByTestId('navigation')
  const currentWidth = navigation.dataset.expanded === 'true'
    ? parseFloat(screen.getByTestId('wrapper').style.getPropertyValue('--sidebar-width')) : 80
  vi.spyOn(navigation, 'getBoundingClientRect').mockReturnValue({ width: currentWidth } as DOMRect)
  fireEvent.pointerDown(screen.getByRole('separator'), { pointerId: 1, isPrimary: true, button: 0, clientX: currentWidth })
}

function moveTo(clientX: number) {
  fireEvent.pointerMove(window, { pointerId: 1, clientX })
}

function releaseAt(clientX: number) {
  fireEvent.pointerUp(window, { pointerId: 1, clientX })
}

describe('primary menu sizing', () => {
  beforeEach(() => localStorage.setItem('nova.layout.activityBarWidth', '256'))

  it('snaps both ways, persists only on release, and retains the expanded width when collapsed', () => {
    render(<Navigation />)
    startDrag()
    moveTo(100)
    expect(screen.getByTestId('navigation')).toHaveAttribute('data-expanded', 'false')
    expect(screen.getByRole('status')).toHaveTextContent('expanded')
    releaseAt(100)
    expect(screen.getByRole('status')).toHaveTextContent('collapsed')
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('256')

    startDrag()
    moveTo(240)
    expect(screen.getByTestId('navigation')).toHaveAttribute('data-expanded', 'true')
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('256')
    releaseAt(240)
    expect(screen.getByRole('status')).toHaveTextContent('expanded')
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('240')
  })

  it.each(['Escape', 'pointercancel', 'blur'])('restores the previous state after %s interrupts a drag', (reason) => {
    render(<Navigation />)
    startDrag()
    moveTo(100)
    if (reason === 'Escape') fireEvent.keyDown(window, { key: reason })
    else if (reason === 'pointercancel') fireEvent.pointerCancel(window, { pointerId: 1 })
    else fireEvent.blur(window)
    expect(screen.getByTestId('navigation')).toHaveAttribute('data-expanded', 'true')
    expect(screen.getByTestId('wrapper').style.getPropertyValue('--sidebar-width')).toBe('256px')
    moveTo(280)
    releaseAt(280)
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('256')
  })

  it('cancels resizing when the desktop navigation is removed at the mobile breakpoint', () => {
    const { rerender } = render(<Navigation />)
    startDrag()
    moveTo(270)
    rerender(<Navigation enabled={false} />)
    releaseAt(280)
    expect(screen.getByTestId('wrapper').style.getPropertyValue('--sidebar-width')).toBe('256px')
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('256')
  })

  it('supports collapsing, restoring and resizing with the keyboard', () => {
    render(<Navigation />)
    const handle = screen.getByRole('separator')
    fireEvent.keyDown(handle, { key: 'Home' })
    expect(screen.getByRole('status')).toHaveTextContent('collapsed')
    fireEvent.keyDown(handle, { key: 'ArrowRight' })
    expect(screen.getByRole('status')).toHaveTextContent('expanded')
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('256')
    fireEvent.keyDown(handle, { key: 'ArrowLeft' })
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('248')
    fireEvent.keyDown(handle, { key: 'Enter' })
    fireEvent.keyDown(handle, { key: 'End' })
    expect(screen.getByRole('status')).toHaveTextContent('expanded')
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('288')
  })

  it('allows a narrow expanded menu and collapses only beyond its minimum', () => {
    render(<Navigation />)
    startDrag()
    moveTo(112)
    expect(screen.getByTestId('navigation')).toHaveAttribute('data-expanded', 'true')
    releaseAt(112)
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('112')
    startDrag()
    moveTo(100)
    releaseAt(100)
    expect(screen.getByRole('status')).toHaveTextContent('collapsed')
    expect(localStorage.getItem('nova.layout.activityBarWidth')).toBe('112')
  })

  it('restores a chosen narrow width without reinterpreting it as a legacy default', () => {
    localStorage.setItem('nova.layout.activityBarWidth', '152')
    render(<Navigation />)
    expect(screen.getByTestId('wrapper').style.getPropertyValue('--sidebar-width')).toBe('152px')
  })
})
