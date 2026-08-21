import { useLayoutEffect, useRef, type KeyboardEventHandler, type PointerEventHandler, type ReactNode } from 'react'
import { Group, Panel, useGroupRef, usePanelRef } from 'react-resizable-panels'
import type { Layout } from 'react-resizable-panels'
import { useTranslation } from 'react-i18next'
import { CollapsiblePanelSeparator, CollapsibleResizablePanel, PanelMotionGroup } from './panel-motion'
import { readPersistedPanelLayout, usePersistedPanelLayout } from './use-persisted-panel-layout'

// 240px keeps three shortcut columns stable after padding and a non-overlay scrollbar;
// narrow screens still let the panel constraints shrink the layout adaptively.
const DEFAULT_SIDEBAR_WIDTH = '240px'
const DEFAULT_SIDEBAR_WIDTH_PX = 240
const COMFORTABLE_SIDEBAR_WIDTH_PX = 224
const MIN_SIDEBAR_WIDTH_PX = 180
const MAX_STORED_SIDEBAR_WIDTH_PX = 8192
const SIDEBAR_WIDTH_STORAGE_KEY = 'nova.layout.workspaceSidebarWidth:v1'

interface WorkspaceLayoutProps {
  activityBar: ReactNode
  topBar?: ReactNode
  sidebar?: ReactNode
  main: ReactNode
  rightPanel?: ReactNode
  bottomPanel?: ReactNode
  statusBar?: ReactNode
  sidebarVisible?: boolean
  rightPanelVisible?: boolean
  bottomPanelVisible?: boolean
  rightPanelWide?: boolean
  centerFocus?: boolean
  /** Route-level layout identity; changes bypass panel motion while explicit pane toggles animate. */
  routeLayoutKey?: string | number
}

/** 工作台布局组件，只负责可拖拽区域编排，不承载业务逻辑。 */
export function WorkspaceLayout({
  activityBar,
  topBar,
  sidebar,
  main,
  rightPanel,
  bottomPanel,
  statusBar,
  sidebarVisible = true,
  rightPanelVisible = true,
  bottomPanelVisible = true,
  rightPanelWide = false,
  centerFocus = false,
  routeLayoutKey,
}: WorkspaceLayoutProps) {
  const { t } = useTranslation()
  const horizontalGroupRef = useGroupRef()
  const sidebarPanelRef = usePanelRef()
  const storedSidebarWidthRef = useRef<number | null | undefined>(undefined)
  if (storedSidebarWidthRef.current === undefined) {
    storedSidebarWidthRef.current = readStoredWorkspaceSidebarWidth()
  }
  const sidebarWidthInitializedRef = useRef(false)
  const rightPanelRef = usePanelRef()
  const rightPanelElementRef = useRef<HTMLDivElement | null>(null)
  const retainedRightPanelRef = useRef<ReactNode>(null)
  const hasRightPanel = rightPanel !== null && rightPanel !== undefined
  if (hasRightPanel) retainedRightPanelRef.current = rightPanel
  const rightPanelOpen = hasRightPanel && rightPanelVisible
  const panelMotionSnapshot = { routeLayoutKey, sidebarVisible, rightPanelOpen, rightPanelWide, centerFocus }
  const previousPanelMotionSnapshotRef = useRef(panelMotionSnapshot)
  const panelMotionSuspendedRef = useRef(false)
  const previousPanelMotionSnapshot = previousPanelMotionSnapshotRef.current
  if (routeLayoutKey !== undefined && previousPanelMotionSnapshot.routeLayoutKey !== routeLayoutKey) {
    // Retained routes already hold their own state. Switching between them should replace the
    // surrounding geometry directly, including the panel library's follow-up layout commits.
    panelMotionSuspendedRef.current = true
  } else if (
    previousPanelMotionSnapshot.sidebarVisible !== sidebarVisible
    || previousPanelMotionSnapshot.rightPanelOpen !== rightPanelOpen
    || previousPanelMotionSnapshot.rightPanelWide !== rightPanelWide
    || previousPanelMotionSnapshot.centerFocus !== centerFocus
  ) {
    // A geometry change inside the current route is an explicit layout action and should remain
    // smooth. Keep this mode until the next route switch so internal panel commits cannot race it.
    panelMotionSuspendedRef.current = false
  }
  previousPanelMotionSnapshotRef.current = panelMotionSnapshot
  const layoutBeforeEmphasisRef = useRef<Layout | null>(null)
  const lastNormalLayoutRef = useRef<Layout | null>(readStoredLayoutForWorkspace('nova-workspace-horizontal', ['sidebar', 'center', 'right']) ?? null)
  const lastRightPanelPixelsRef = useRef<number | null>(null)
  const centerWidthReadyRef = useRef(false)
  const previousEmphasisRef = useRef<'normal' | 'right' | 'center'>('normal')
  const layoutEmphasis = rightPanelWide ? 'right' : centerFocus ? 'center' : 'normal'
  const layoutEmphasisRef = useRef(layoutEmphasis)
  layoutEmphasisRef.current = layoutEmphasis
  const horizontalPanelLayout = usePersistedPanelLayout({
    storageKey: 'nova-workspace-horizontal',
    panelIds: sidebar ? ['sidebar', 'center', 'right'] : ['center', 'right'],
  })
  const verticalPanelLayout = usePersistedPanelLayout({
    storageKey: 'nova-workspace-main-vertical',
    panelIds: bottomPanelVisible && bottomPanel ? ['main', 'bottom'] : ['main'],
  })

  useLayoutEffect(() => {
    if (!sidebar || !sidebarVisible || sidebarWidthInitializedRef.current) return
    const panel = sidebarPanelRef.current
    if (!panel) return

    const currentWidth = panel.getSize().inPixels
    const targetWidth = resolveInitialWorkspaceSidebarWidth(storedSidebarWidthRef.current ?? null, currentWidth)
    if (Math.abs(currentWidth - targetWidth) > 1) panel.resize(`${targetWidth}px`)
    if (storedSidebarWidthRef.current === null) {
      persistWorkspaceSidebarWidth(targetWidth)
      storedSidebarWidthRef.current = targetWidth
    }
    sidebarWidthInitializedRef.current = true
  }, [sidebar, sidebarPanelRef, sidebarVisible])

  useLayoutEffect(() => {
    if (!rightPanelOpen || layoutEmphasis !== 'normal' || previousEmphasisRef.current !== 'normal') return
    const width = rightPanelElementRef.current?.getBoundingClientRect().width ?? 0
    if (width > 0) lastRightPanelPixelsRef.current = width
  }, [layoutEmphasis, rightPanelOpen])

  useLayoutEffect(() => {
    if (!rightPanelOpen) {
      layoutBeforeEmphasisRef.current = null
      centerWidthReadyRef.current = false
      previousEmphasisRef.current = 'normal'
      return
    }

    const updateRightPanelWidth = () => {
      const group = horizontalGroupRef.current
      if (!group) return
      const layout = group.getLayout()
      if (typeof layout.right !== 'number' || typeof layout.center !== 'number') return

      if (layoutEmphasis === 'normal') {
        const returningFromCenter = previousEmphasisRef.current === 'center'
        const retainedPixels = lastRightPanelPixelsRef.current
        const storedLayout = previousEmphasisRef.current === 'right' ? layoutBeforeEmphasisRef.current : null
        layoutBeforeEmphasisRef.current = null
        centerWidthReadyRef.current = false
        previousEmphasisRef.current = 'normal'
        if (storedLayout && typeof storedLayout.right === 'number' && typeof storedLayout.center === 'number'
          && (Math.abs(storedLayout.right - layout.right) > 1 || Math.abs(storedLayout.center - layout.center) > 1)) {
          group.setLayout(storedLayout)
        }
        if (returningFromCenter && retainedPixels !== null) rightPanelRef.current?.resize(retainedPixels)
        return
      }

      if (layoutEmphasis === 'center') {
        const enteringCenter = previousEmphasisRef.current !== 'center'
        const retainedPixels = lastRightPanelPixelsRef.current
        const storedLayout = previousEmphasisRef.current === 'right' ? layoutBeforeEmphasisRef.current : null
        layoutBeforeEmphasisRef.current = null
        previousEmphasisRef.current = 'center'
        if (storedLayout && typeof storedLayout.right === 'number' && typeof storedLayout.center === 'number'
          && (Math.abs(storedLayout.right - layout.right) > 1 || Math.abs(storedLayout.center - layout.center) > 1)) {
          group.setLayout(storedLayout)
        }
        centerWidthReadyRef.current = true
        if (enteringCenter && retainedPixels !== null) rightPanelRef.current?.resize(retainedPixels)
        return
      }

      centerWidthReadyRef.current = false
      if (previousEmphasisRef.current !== 'right' && !layoutBeforeEmphasisRef.current) {
        layoutBeforeEmphasisRef.current = lastNormalLayoutRef.current ?? layout
      }
      previousEmphasisRef.current = 'right'
      const nextLayout = resizeRightPanel(layout, 58, sidebarVisible)
      const layoutSum = Object.values(layout).reduce((sum, value) => sum + value, 0)
      if (Math.abs((nextLayout.right ?? 0) - layout.right) > 1
        || Math.abs((nextLayout.center ?? 0) - layout.center) > 1
        || Math.abs(layoutSum - 100) > 1) {
        group.setLayout(nextLayout)
      }
    }
    updateRightPanelWidth()
    const frame = window.requestAnimationFrame(updateRightPanelWidth)
    return () => window.cancelAnimationFrame(frame)
  }, [horizontalGroupRef, layoutEmphasis, rightPanelOpen, sidebarVisible])

  return (
    <div data-nova-app-shell="true" className="h-dvh w-screen overflow-hidden">
      <div className="flex h-full flex-col">
        {topBar}
        <div className="flex min-h-0 flex-1">
          {activityBar}
          <PanelMotionGroup
            id="nova-workspace-horizontal"
            motionSuspended={panelMotionSuspendedRef.current}
            data-nova-layout-emphasis={layoutEmphasis}
            groupRef={horizontalGroupRef}
            defaultLayout={horizontalPanelLayout.defaultLayout}
            onLayoutChanged={(layout) => {
              if (layoutEmphasis !== 'normal' || (sidebar && !sidebarVisible)) return
              let normalLayout = layout
              if (!rightPanelOpen) {
                const retainedRightSize = lastNormalLayoutRef.current?.right
                  ?? (typeof layout.right === 'number' && layout.right > 0 ? layout.right : undefined)
                if (typeof retainedRightSize !== 'number') return
                const sidebarSize = typeof layout.sidebar === 'number' ? layout.sidebar : 0
                normalLayout = {
                  ...layout,
                  center: Math.max(100 - sidebarSize - retainedRightSize, 0),
                  right: retainedRightSize,
                }
              }
              if (horizontalPanelLayout.persistUserLayout(normalLayout)) lastNormalLayoutRef.current = normalLayout
            }}
            orientation="horizontal"
            resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
            className="min-w-0 flex-1"
          >
            {sidebar && (
              <>
                <CollapsibleResizablePanel
                  id="sidebar"
                  panelRef={sidebarPanelRef}
                  visible={sidebarVisible}
                  side="left"
                  initialExpandSize={`${storedSidebarWidthRef.current ?? DEFAULT_SIDEBAR_WIDTH_PX}px`}
                  defaultSize={DEFAULT_SIDEBAR_WIDTH}
                  minSize="180px"
                  maxSize="36%"
                  groupResizeBehavior="preserve-pixel-size"
                  className="min-w-[180px]"
                  data-nova-collapsible-panel="sidebar"
                  onResize={(size) => {
                    if (!horizontalPanelLayout.isUserResizeActive() || !Number.isFinite(size.inPixels) || size.inPixels <= 0) return
                    persistWorkspaceSidebarWidth(size.inPixels)
                    storedSidebarWidthRef.current = size.inPixels
                  }}
                >
                  {sidebar}
                </CollapsibleResizablePanel>
                <WorkspaceResizeHandle
                  visible={sidebarVisible}
                  direction="vertical"
                  label={t('layout.resize.sidebar')}
                  {...horizontalPanelLayout.resizeHandleIntentProps}
                />
              </>
            )}
            <Panel id="center" minSize={rightPanelWide ? '260px' : '30%'} className="min-w-0">
              <Group
                id="nova-workspace-main-vertical"
                disableCursor
                defaultLayout={verticalPanelLayout.defaultLayout}
                onLayoutChanged={(layout) => {
                  verticalPanelLayout.persistUserLayout(layout)
                }}
                orientation="vertical"
                resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
              >
                <Panel id="main" minSize="35%" className="min-h-0">
                  {main}
                </Panel>
                {bottomPanelVisible && bottomPanel && (
                  <>
                    <WorkspaceResizeHandle
                      direction="horizontal"
                      label={t('layout.resize.bottom')}
                      {...verticalPanelLayout.resizeHandleIntentProps}
                    />
                    <Panel id="bottom" defaultSize="18%" minSize="96px" maxSize="40%" className="min-h-[96px]">
                      {bottomPanel}
                    </Panel>
                  </>
                )}
              </Group>
            </Panel>
            <WorkspaceResizeHandle
              visible={rightPanelOpen}
              direction="vertical"
              label={t('layout.resize.right')}
              {...horizontalPanelLayout.resizeHandleIntentProps}
            />
            <CollapsibleResizablePanel
              id="right"
              panelRef={rightPanelRef}
              elementRef={rightPanelElementRef}
              visible={rightPanelOpen}
              side="right"
              defaultSize={rightPanelWide ? '58%' : '34%'}
              minSize={rightPanelWide ? '520px' : '360px'}
              maxSize={rightPanelWide ? '68%' : '55%'}
              groupResizeBehavior="preserve-pixel-size"
              className={rightPanelWide ? 'min-w-[520px]' : 'min-w-[360px]'}
              data-nova-right-panel={rightPanelWide ? 'wide' : 'default'}
              data-nova-resize-behavior="preserve-pixel-size"
              onResize={(size) => {
                const emphasis = layoutEmphasisRef.current
                const stableNormal = emphasis === 'normal' && previousEmphasisRef.current === 'normal'
                const adjustableReview = emphasis === 'center' && centerWidthReadyRef.current
                if ((stableNormal || adjustableReview) && size.inPixels > 0) lastRightPanelPixelsRef.current = size.inPixels
              }}
            >
              {retainedRightPanelRef.current}
            </CollapsibleResizablePanel>
          </PanelMotionGroup>
        </div>
        {statusBar}
      </div>
    </div>
  )
}

function resizeRightPanel(current: Layout, rightSize: number, sidebarVisible: boolean): Layout {
  const sidebarSize = sidebarVisible && typeof current.sidebar === 'number' ? current.sidebar : 0
  return {
    ...current,
    ...(typeof current.sidebar === 'number' ? { sidebar: sidebarSize } : {}),
    center: Math.max(100 - sidebarSize - rightSize, 22),
    right: rightSize,
  }
}

function WorkspaceResizeHandle({
  direction,
  label,
  visible = true,
  onPointerDownCapture,
  onKeyDownCapture,
}: {
  direction: 'horizontal' | 'vertical'
  label: string
  visible?: boolean
  onPointerDownCapture?: PointerEventHandler<HTMLElement>
  onKeyDownCapture?: KeyboardEventHandler<HTMLElement>
}) {
  const className = direction === 'vertical'
    ? 'nova-resize-handle nova-resize-divider nova-resize-divider-vertical relative z-30 -mx-1 w-2 shrink-0 touch-none cursor-col-resize select-none'
    : 'nova-resize-handle nova-resize-divider nova-resize-divider-horizontal relative z-30 -my-1 h-2 shrink-0 touch-none cursor-row-resize select-none'

  return (
    <CollapsiblePanelSeparator
      visible={visible}
      aria-label={label}
      className={className}
      onPointerDownCapture={onPointerDownCapture}
      onKeyDownCapture={onKeyDownCapture}
    />
  )
}

export function readStoredLayoutForWorkspace(key: string, panelOrder?: string[]): Layout | undefined {
  return readPersistedPanelLayout(key, panelOrder)
}

/** Resolves the one-time migration from a legacy narrow percentage to pixel persistence. */
export function resolveInitialWorkspaceSidebarWidth(storedWidth: number | null, currentWidth: number): number {
  if (storedWidth !== null) return storedWidth
  return currentWidth >= COMFORTABLE_SIDEBAR_WIDTH_PX ? currentWidth : DEFAULT_SIDEBAR_WIDTH_PX
}

function readStoredWorkspaceSidebarWidth(): number | null {
  if (typeof window === 'undefined') return null
  try {
    const raw = window.localStorage.getItem(SIDEBAR_WIDTH_STORAGE_KEY)
    if (raw === null) return null
    const width = Number(raw)
    return Number.isFinite(width) && width >= MIN_SIDEBAR_WIDTH_PX && width <= MAX_STORED_SIDEBAR_WIDTH_PX
      ? width
      : null
  } catch (error) {
    console.warn('[workspace-layout] Unable to restore the Writing sidebar width', { error })
    return null
  }
}

function persistWorkspaceSidebarWidth(width: number) {
  if (typeof window === 'undefined' || !Number.isFinite(width) || width <= 0) return
  try {
    window.localStorage.setItem(SIDEBAR_WIDTH_STORAGE_KEY, String(Math.round(width)))
  } catch (error) {
    console.warn('[workspace-layout] Unable to persist the Writing sidebar width', { error })
  }
}
