import { render, waitFor } from '@testing-library/react'
import { useLayoutEffect, useRef, type ReactNode, type RefObject } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PanelImperativeHandle, PanelSize } from 'react-resizable-panels'
import { CollapsibleResizablePanel } from './panel-motion'

const panelHarness = vi.hoisted(() => ({
  reportedPixels: 368,
  collapse: vi.fn(),
  expand: vi.fn(),
  resize: vi.fn(),
}))

vi.mock('react-resizable-panels', () => ({
  Panel: ({
    children,
    id,
    onResize,
    panelRef,
  }: {
    children: ReactNode
    id?: string
    onResize?: (size: PanelSize, id: string | undefined, previousSize: PanelSize | undefined) => void
    panelRef?: RefObject<PanelImperativeHandle | null>
  }) => {
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
})

function panel(visible: boolean) {
  return (
    <CollapsibleResizablePanel
      id="sidebar"
      visible={visible}
      side="left"
      defaultSize="224px"
      minSize="180px"
    >
      Sidebar
    </CollapsibleResizablePanel>
  )
}
