import type { ReactNode } from 'react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import type { Layout } from 'react-resizable-panels'

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
            className="min-w-0 flex-1"
          >
            {sidebarVisible && sidebar && (
              <>
                <Panel id="sidebar" defaultSize="20%" minSize="180px" maxSize="36%" className="min-w-[180px]">
                  {sidebar}
                </Panel>
                <WorkspaceResizeHandle direction="vertical" label="调整项目结构宽度" />
              </>
            )}
            <Panel id="center" minSize="30%" className="min-w-0">
              <Group
                id="nova-workspace-main-vertical"
                defaultLayout={readStoredLayout('nova-workspace-main-vertical')}
                onLayoutChanged={(layout) => storeLayout('nova-workspace-main-vertical', layout)}
                orientation="vertical"
              >
                <Panel id="main" minSize="35%" className="min-h-0">
                  {main}
                </Panel>
                {bottomPanelVisible && bottomPanel && (
                  <>
                    <WorkspaceResizeHandle direction="horizontal" label="调整任务面板高度" />
                    <Panel id="bottom" defaultSize="18%" minSize="96px" maxSize="40%" className="min-h-[96px]">
                      {bottomPanel}
                    </Panel>
                  </>
                )}
              </Group>
            </Panel>
            {rightPanelVisible && rightPanel && (
              <>
                <WorkspaceResizeHandle direction="vertical" label="调整右侧面板宽度" />
                <Panel id="right" defaultSize="28%" minSize="300px" maxSize="45%" className="min-w-[300px]">
                  {rightPanel}
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
