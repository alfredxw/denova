import type { ReactNode } from 'react'
import { Group, Panel, Separator } from 'react-resizable-panels'
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
}: WorkspaceLayoutProps) {
  const { t } = useTranslation()
  return (
    <div data-nova-app-shell="true" className="h-screen w-screen overflow-hidden">
      <div className="flex h-full flex-col">
        {topBar}
        <div className="flex min-h-0 flex-1">
          {activityBar}
          <Group
            id="nova-workspace-horizontal"
            defaultLayout={readStoredLayout('nova-workspace-horizontal')}
            onLayoutChanged={(layout) => storeLayout('nova-workspace-horizontal', layout)}
            orientation="horizontal"
            resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
            className="min-w-0 flex-1"
          >
            {sidebar && (
              <>
                <Panel id="sidebar" defaultSize="20%" minSize="180px" maxSize="36%" className="min-w-[180px]" disabled={!sidebarVisible} hidden={!sidebarVisible} aria-hidden={!sidebarVisible}>
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
            <Panel id="center" minSize="30%" className="min-w-0">
              <Group
                id="nova-workspace-main-vertical"
                defaultLayout={readStoredLayout('nova-workspace-main-vertical')}
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
                <Panel id="right" defaultSize="34%" minSize="360px" maxSize="55%" className="min-w-[360px]" disabled={!rightPanelVisible} hidden={!rightPanelVisible} aria-hidden={!rightPanelVisible}>
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

function WorkspaceResizeHandle({ direction, label }: { direction: 'horizontal' | 'vertical'; label: string }) {
  const className = direction === 'vertical'
    ? 'nova-resize-handle -mx-1 w-2 cursor-col-resize bg-transparent transition-colors'
    : 'nova-resize-handle -my-1 h-2 cursor-row-resize bg-transparent transition-colors'

  return <Separator aria-label={label} className={className} />
}

function readStoredLayout(key: string): Layout | undefined {
  if (typeof window === 'undefined') return undefined
  const value = window.localStorage.getItem(key)
  if (!value) return undefined
  try {
    return JSON.parse(value) as Layout
  } catch {
    return undefined
  }
}

function storeLayout(key: string, layout: Layout) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(key, JSON.stringify(layout))
}
