import { fireEvent, render, waitFor } from '@testing-library/react'
import { useLayoutEffect, useRef, type HTMLAttributes, type ReactNode, type RefObject } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PanelImperativeHandle, PanelSize } from 'react-resizable-panels'
import { CollapsibleResizablePanel, PanelMotionGroup } from './panel-motion'

const panelHarness = vi.hoisted(() => ({
  reportedPixels: 368,
  groupRenders: 0,
  groupDisableCursor: false,
  collapse: vi.fn(),
  expand: vi.fn(),
  resize: vi.fn(),
  renderedDefaultSize: undefined as number | string | undefined,
}))

vi.mock('react-resizable-panels', () => ({
  Group: ({ children, orientation: _orientation, disableCursor, ...props }: HTMLAttributes<HTMLDivElement> & {
    children?: ReactNode
    orientation?: 'horizontal' | 'vertical'
    disableCursor?: boolean
  }) => {
    panelHarness.groupRenders += 1
    panelHarness.groupDisableCursor = disableCursor === true
    return <div {...props}>{children}</div>
  },
  Panel: ({
    children,
    id,
    onResize,
    panelRef,
    defaultSize,
  }: {
    children: ReactNode
    id?: string
    onResize?: (size: PanelSize, id: string | undefined, previousSize: PanelSize | undefined) => void
    panelRef?: RefObject<PanelImperativeHandle | null>
    defaultSize?: number | string
  }) => {
    panelHarness.renderedDefaultSize = defaultSize
    useLayoutEffect(() => {
      if (panelRef) {
        panelRef.current = {
          collapse: panelHarness.collapse,
          expand: panelHarness.expand,
          getSize: () => ({ asPercentage: 0, inPixels: panelHarness.reportedPixels }),
          isCollapsed: () => false,
          resize: panelHarness.resize,
        }
      }
      onResize?.({ asPercentage: 30, inPixels: panelHarness.reportedPixels }, id, undefined)
      return () => {
        if (panelRef) panelRef.current = null
      }
    }, [id, onResize, panelRef])
    return <div id={id}>{children}</div>
  },
  Separator: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  usePanelRef: () => useRef<PanelImperativeHandle | null>(null),
}))

describe('CollapsibleResizablePanel', () => {
  beforeEach(() => {
    panelHarness.reportedPixels = 368
    panelHarness.collapse.mockReset()
    panelHarness.expand.mockReset()
    panelHarness.resize.mockReset()
    panelHarness.renderedDefaultSize = undefined
  })

  it('restores its last visible pixel width after a programmatic collapse', async () => {
    const { rerender } = render(panel(true))

    rerender(panel(false))
    await waitFor(() => expect(panelHarness.collapse).toHaveBeenCalledTimes(1))

    // A collapsed panel may report zero before it becomes visible again; that must not replace
    // the last useful width captured while the pane was open.
    panelHarness.reportedPixels = 0
    rerender(panel(true))

    await waitFor(() => {
      expect(panelHarness.expand).toHaveBeenCalledTimes(1)
      expect(panelHarness.resize).toHaveBeenCalledWith(368)
    })
  })

  it('restores the configured layout when the panel starts hidden', async () => {
    const { rerender } = render(panel(false))
    expect(panelHarness.renderedDefaultSize).toBe('0px')
    await waitFor(() => expect(panelHarness.collapse).toHaveBeenCalledTimes(1))

    panelHarness.reportedPixels = 0
    rerender(panel(true))

    await waitFor(() => {
      expect(panelHarness.expand).toHaveBeenCalledTimes(1)
      expect(panelHarness.resize).toHaveBeenCalledWith('224px')
    })
  })

  it('restores a persisted percentage when the panel starts hidden', async () => {
    const { rerender } = render(panel(false, { initialExpandSize: '37%', restorationKey: 'project-a' }))
    await waitFor(() => expect(panelHarness.collapse).toHaveBeenCalledTimes(1))

    rerender(panel(true, { initialExpandSize: '37%', restorationKey: 'project-a' }))

    await waitFor(() => {
      expect(panelHarness.expand).toHaveBeenCalledTimes(1)
      expect(panelHarness.resize).toHaveBeenCalledWith('37%')
    })
  })

  it('applies the target layout when the restoration context changes while visible', async () => {
    const { rerender } = render(panel(true, { initialExpandSize: '50%', restorationKey: 'project-a' }))
    panelHarness.resize.mockClear()

    rerender(panel(true, { initialExpandSize: '42%', restorationKey: 'project-b' }))

    await waitFor(() => expect(panelHarness.resize).toHaveBeenCalledWith('42%'))
  })
})

describe('PanelMotionGroup', () => {
  beforeEach(() => {
    panelHarness.groupRenders = 0
    panelHarness.groupDisableCursor = false
  })

  it('disables the library-wide cursor stylesheet', () => {
    render(
      <PanelMotionGroup orientation="horizontal">
        <div />
      </PanelMotionGroup>,
    )

    expect(panelHarness.groupDisableCursor).toBe(true)
  })

  it('exposes an explicit panel-motion suspension state', () => {
    const { container, rerender } = render(
      <PanelMotionGroup orientation="horizontal" motionSuspended>
        <div />
      </PanelMotionGroup>,
    )
    const group = container.firstElementChild

    expect(group).toHaveAttribute('data-nova-panel-motion-suspended', 'true')

    rerender(
      <PanelMotionGroup orientation="horizontal">
        <div />
      </PanelMotionGroup>,
    )
    expect(group).not.toHaveAttribute('data-nova-panel-motion-suspended')
  })

  it('tracks only its direct separator resize without a relational CSS selector', async () => {
    const { getByTestId } = render(
      <PanelMotionGroup orientation="horizontal">
        <div data-separator="inactive" data-testid="resize-separator" />
      </PanelMotionGroup>,
    )
    const separator = getByTestId('resize-separator')
    const group = separator.parentElement

    expect(group).toHaveAttribute('data-nova-panel-motion-group', 'true')
    expect(group).not.toHaveAttribute('data-nova-panel-resizing')

    fireEvent.pointerDown(separator)
    expect(group).toHaveAttribute('data-nova-panel-resizing', 'true')
    expect(panelHarness.groupRenders).toBe(1)

    fireEvent.pointerUp(window)
    await waitFor(() => expect(group).not.toHaveAttribute('data-nova-panel-resizing'))
    expect(panelHarness.groupRenders).toBe(1)
  })

  it('does not mark an ancestor panel group while a nested group is resizing', () => {
    const { getByTestId } = render(
      <PanelMotionGroup orientation="horizontal">
        <PanelMotionGroup orientation="horizontal">
          <div data-separator="inactive" data-testid="nested-resize-separator" />
        </PanelMotionGroup>
      </PanelMotionGroup>,
    )
    const innerGroup = getByTestId('nested-resize-separator').parentElement
    const outerGroup = innerGroup?.parentElement

    fireEvent.pointerDown(getByTestId('nested-resize-separator'))

    expect(innerGroup).toHaveAttribute('data-nova-panel-resizing', 'true')
    expect(outerGroup).not.toHaveAttribute('data-nova-panel-resizing')
  })

  it('tracks a resize after the panel library prevents the native pointer default', async () => {
    const preventNativeDefault = (event: PointerEvent) => event.preventDefault()
    document.addEventListener('pointerdown', preventNativeDefault, true)

    try {
      const { getByTestId } = render(
        <PanelMotionGroup orientation="horizontal">
          <div data-separator="inactive" data-testid="prevented-resize-separator" />
        </PanelMotionGroup>,
      )
      const separator = getByTestId('prevented-resize-separator')
      const group = separator.parentElement

      fireEvent.pointerDown(separator)
      expect(group).toHaveAttribute('data-nova-panel-resizing', 'true')

      fireEvent.pointerUp(window)
      await waitFor(() => expect(group).not.toHaveAttribute('data-nova-panel-resizing'))
    } finally {
      document.removeEventListener('pointerdown', preventNativeDefault, true)
    }
  })
})

function panel(
  visible: boolean,
  {
    initialExpandSize = '224px',
    restorationKey = 'test-sidebar-layout',
  }: {
    initialExpandSize?: string
    restorationKey?: string
  } = {},
) {
  return (
    <CollapsibleResizablePanel
      id="sidebar"
      visible={visible}
      side="left"
      initialExpandSize={initialExpandSize}
      restorationKey={restorationKey}
      defaultSize="224px"
      minSize="180px"
    >
      Sidebar
    </CollapsibleResizablePanel>
  )
}
