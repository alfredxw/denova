import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { WorkspaceLayout, readStoredLayoutForWorkspace } from './workspace-layout'

describe('WorkspaceLayout', () => {
  it('removes the sidebar resize target when the sidebar is hidden', () => {
    const { container, rerender } = renderWorkspaceLayout(true)

    expect(container.querySelector('#sidebar')).toBeInTheDocument()
    expect(screen.getByRole('separator', { name: '调整项目结构宽度' })).toHaveClass('cursor-col-resize')

    rerender(workspaceLayout(false))

    expect(container.querySelector('#sidebar')).toHaveAttribute('data-disabled', 'true')
    expect(container.querySelector('#sidebar')).not.toBeVisible()
    expect(screen.queryByRole('separator', { name: '调整项目结构宽度' })).not.toBeInTheDocument()
  })

  it('removes the right panel resize target when the right panel is hidden', () => {
    const { container, rerender } = render(workspaceLayoutWithRightPanel(true))

    expect(container.querySelector('#right')).toBeInTheDocument()
    expect(screen.getByRole('separator', { name: '调整右侧面板宽度' })).toHaveClass('cursor-col-resize')

    rerender(workspaceLayoutWithRightPanel(false))

    expect(container.querySelector('#right')).toHaveAttribute('data-disabled', 'true')
    expect(container.querySelector('#right')).not.toBeVisible()
    expect(screen.queryByRole('separator', { name: '调整右侧面板宽度' })).not.toBeInTheDocument()
  })

  it('marks the right panel wide variant for detail-heavy content', () => {
    const { container } = render(
      <WorkspaceLayout
        activityBar={<nav aria-label="一级菜单栏">菜单</nav>}
        main={<main>正文区域</main>}
        rightPanel={<aside>创作 Agent</aside>}
        rightPanelWide
      />,
    )

    expect(container.querySelector('#right')).toHaveAttribute('data-nova-right-panel', 'wide')
  })

  it('normalizes persisted workspace layout order before handing it to resizable panels', () => {
    window.localStorage.setItem('nova-workspace-horizontal', JSON.stringify({ right: 34, center: 46, sidebar: 20 }))

    const layout = readStoredLayoutForWorkspace('nova-workspace-horizontal', ['sidebar', 'center', 'right'])

    expect(Object.keys(layout || {})).toEqual(['sidebar', 'center', 'right'])
    expect(layout).toEqual({ sidebar: 20, center: 46, right: 34 })
  })
})

function renderWorkspaceLayout(sidebarVisible: boolean) {
  return render(workspaceLayout(sidebarVisible))
}

function workspaceLayout(sidebarVisible: boolean) {
  return (
    <WorkspaceLayout
      activityBar={<nav aria-label="一级菜单栏">菜单</nav>}
      sidebar={<div>项目结构</div>}
      sidebarVisible={sidebarVisible}
      main={<main>正文区域</main>}
    />
  )
}

function workspaceLayoutWithRightPanel(rightPanelVisible: boolean) {
  return (
    <WorkspaceLayout
      activityBar={<nav aria-label="一级菜单栏">菜单</nav>}
      main={<main>正文区域</main>}
      rightPanel={<aside>创作 Agent</aside>}
      rightPanelVisible={rightPanelVisible}
    />
  )
}
