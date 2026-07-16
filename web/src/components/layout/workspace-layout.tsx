import { useLayoutEffect, useRef, type ReactNode } from 'react'
import { Group, Panel, Separator, useGroupRef, usePanelRef } from 'react-resizable-panels'
import type { Layout } from 'react-resizable-panels'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { novaEase, subtlePresence } from '@/features/motion/motion-tokens'

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
}: WorkspaceLayoutProps) {
  const { t } = useTranslation()
  const horizontalGroupRef = useGroupRef()
  const sidebarPanelRef = usePanelRef()
  const rightPanelRef = usePanelRef()
  const rightPanelElementRef = useRef<HTMLDivElement | null>(null)
  const layoutBeforeEmphasisRef = useRef<Layout | null>(null)
  const lastNormalLayoutRef = useRef<Layout | null>(readStoredLayoutForWorkspace('nova-workspace-horizontal', ['sidebar', 'center', 'right']) ?? null)
  const lastRightPanelPixelsRef = useRef<number | null>(null)
  const centerWidthReadyRef = useRef(false)
  const previousEmphasisRef = useRef<'normal' | 'right' | 'center'>('normal')
  const layoutEmphasis = rightPanelWide ? 'right' : centerFocus ? 'center' : 'normal'
  const layoutEmphasisRef = useRef(layoutEmphasis)
  layoutEmphasisRef.current = layoutEmphasis

  useLayoutEffect(() => {
    const panel = sidebarPanelRef.current
    if (!panel) return
    if (sidebarVisible) panel.expand()
    else panel.collapse()
  }, [sidebarVisible])

  useLayoutEffect(() => {
    if (!rightPanelVisible || layoutEmphasis !== 'normal' || previousEmphasisRef.current !== 'normal') return
    const width = rightPanelElementRef.current?.getBoundingClientRect().width ?? 0
    if (width > 0) lastRightPanelPixelsRef.current = width
  }, [layoutEmphasis, rightPanelVisible])

  useLayoutEffect(() => {
    if (!rightPanelVisible) {
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
  }, [horizontalGroupRef, layoutEmphasis, rightPanelVisible, sidebarVisible])

  return (
    <div data-nova-app-shell="true" className="h-dvh w-screen overflow-hidden">
      <div className="flex h-full flex-col">
        {topBar}
        <div className="flex min-h-0 flex-1">
          {activityBar}
          <Group
            id="nova-workspace-horizontal"
            data-nova-layout-emphasis={layoutEmphasis}
            groupRef={horizontalGroupRef}
            defaultLayout={readStoredLayoutForWorkspace('nova-workspace-horizontal', ['sidebar', 'center', 'right'])}
            onLayoutChanged={(layout) => {
              if (layoutEmphasis === 'normal' && (!sidebar || sidebarVisible)) {
                lastNormalLayoutRef.current = layout
                storeLayout('nova-workspace-horizontal', layout)
              }
            }}
            orientation="horizontal"
            resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
            className="min-w-0 flex-1"
          >
            {sidebar && (
              <>
                <Panel
                  id="sidebar"
                  panelRef={sidebarPanelRef}
                  defaultSize="20%"
                  minSize="180px"
                  maxSize="36%"
                  collapsedSize="0px"
                  collapsible
                  className="min-w-[180px]"
                  disabled={!sidebarVisible}
                  hidden={!sidebarVisible}
                  aria-hidden={!sidebarVisible}
                  data-nova-collapsible-panel="sidebar"
                >
                  <motion.div
                    className="h-full min-h-0"
                    variants={subtlePresence}
                    initial="initial"
                    animate="animate"
                    transition={{ duration: 0.16, ease: novaEase }}
                  >
                    {sidebar}
                  </motion.div>
                </Panel>
                {sidebarVisible ? <WorkspaceResizeHandle direction="vertical" label={t('layout.resize.sidebar')} /> : null}
              </>
            )}
            <Panel id="center" minSize={rightPanelWide ? '260px' : '30%'} className="min-w-0">
              <Group
                id="nova-workspace-main-vertical"
                defaultLayout={readStoredLayoutForWorkspace('nova-workspace-main-vertical', ['main', 'bottom'])}
                onLayoutChanged={(layout) => storeLayout('nova-workspace-main-vertical', layout)}
                orientation="vertical"
                resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
              >
                <Panel id="main" minSize="35%" className="min-h-0">
                  {main}
                </Panel>
                {bottomPanelVisible && bottomPanel && (
                  <>
                    <WorkspaceResizeHandle direction="horizontal" label={t('layout.resize.bottom')} />
                    <Panel id="bottom" defaultSize="18%" minSize="96px" maxSize="40%" className="min-h-[96px]">
                      {bottomPanel}
                    </Panel>
                  </>
                )}
              </Group>
            </Panel>
            {rightPanel && (
              <>
                {rightPanelVisible ? <WorkspaceResizeHandle direction="vertical" label={t('layout.resize.right')} /> : null}
                <Panel
                  id="right"
                  panelRef={rightPanelRef}
                  elementRef={rightPanelElementRef}
                  defaultSize={rightPanelWide ? '58%' : '34%'}
                  minSize={rightPanelWide ? '520px' : '360px'}
                  maxSize={rightPanelWide ? '68%' : '55%'}
                  groupResizeBehavior="preserve-pixel-size"
                  className={rightPanelWide ? 'min-w-[520px]' : 'min-w-[360px]'}
                  disabled={!rightPanelVisible}
                  hidden={!rightPanelVisible}
                  aria-hidden={!rightPanelVisible}
                  data-nova-right-panel={rightPanelWide ? 'wide' : 'default'}
                  data-nova-resize-behavior="preserve-pixel-size"
                  onResize={(size) => {
                    const emphasis = layoutEmphasisRef.current
                    const stableNormal = emphasis === 'normal' && previousEmphasisRef.current === 'normal'
                    const adjustableReview = emphasis === 'center' && centerWidthReadyRef.current
                    if ((stableNormal || adjustableReview) && size.inPixels > 0) lastRightPanelPixelsRef.current = size.inPixels
                  }}
                >
                  <motion.div
                    className="h-full min-h-0"
                    variants={subtlePresence}
                    initial="initial"
                    animate="animate"
                    transition={{ duration: 0.16, ease: novaEase }}
                  >
                    {rightPanel}
                  </motion.div>
                </Panel>
              </>
            )}
          </Group>
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

function WorkspaceResizeHandle({ direction, label }: { direction: 'horizontal' | 'vertical'; label: string }) {
  const className = direction === 'vertical'
    ? 'nova-resize-handle relative z-30 -mx-1 w-2 shrink-0 touch-none cursor-col-resize select-none bg-transparent transition-colors'
    : 'nova-resize-handle relative z-30 -my-1 h-2 shrink-0 touch-none cursor-row-resize select-none bg-transparent transition-colors'

  return <Separator aria-label={label} className={className} />
}

export function readStoredLayoutForWorkspace(key: string, panelOrder?: string[]): Layout | undefined {
  if (typeof window === 'undefined') return undefined
  const value = window.localStorage.getItem(key)
  if (!value) return undefined
  try {
    const layout = JSON.parse(value) as Layout
    if (!panelOrder) return layout
    return panelOrder.reduce<Layout>((ordered, panelId) => {
      if (typeof layout[panelId] === 'number') ordered[panelId] = layout[panelId]
      return ordered
    }, {})
  } catch {
    return undefined
  }
}

function storeLayout(key: string, layout: Layout) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(key, JSON.stringify(layout))
}
