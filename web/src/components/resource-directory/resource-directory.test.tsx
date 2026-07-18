import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Bot } from 'lucide-react'
import { describe, expect, it, vi } from 'vitest'
import { ResourceDirectory } from './ResourceDirectory'
import type { ResourceDirectorySection } from './types'

function buildSections(): ResourceDirectorySection[] {
  return [
    {
      id: 'characters',
      label: '角色',
      items: [
        { id: 'c1', title: '艾拉', summary: '主角' },
        { id: 'c2', title: '凯尔' },
      ],
    },
    {
      id: 'locations',
      label: '地点',
      items: [],
    },
    {
      id: 'rules',
      label: '规则',
      items: [{ id: 'r1', title: '世界法则', badges: [{ label: '常驻' }] }],
      defaultCollapsed: true,
    },
  ]
}

describe('ResourceDirectory', () => {
  it('renders sections with counts and collapses empty sections by default', () => {
    render(<ResourceDirectory sections={buildSections()} activeId={null} onSelect={() => {}} />)

    expect(screen.getByText('角色')).toBeInTheDocument()
    expect(screen.getByText('艾拉')).toBeInTheDocument()
    expect(screen.getByText('凯尔')).toBeInTheDocument()
    expect(screen.getByText('地点')).toBeInTheDocument()
    // 空分组计数为 0
    expect(screen.getByText('0')).toBeInTheDocument()
    // defaultCollapsed 分组不渲染条目
    expect(screen.queryByText('世界法则')).not.toBeInTheDocument()
  })

  it('selects items and marks the active row', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(<ResourceDirectory sections={buildSections()} activeId="c1" onSelect={onSelect} />)

    await user.click(screen.getByText('凯尔'))
    expect(onSelect).toHaveBeenCalledWith('c2')
    expect(screen.getByText('艾拉').closest('button')).toHaveClass('is-active')
  })

  it('filters items by search and expands collapsed sections while searching', async () => {
    const user = userEvent.setup()
    render(<ResourceDirectory sections={buildSections()} activeId={null} onSelect={() => {}} />)

    await user.type(screen.getByLabelText('搜索'), '法则')
    expect(screen.getByText('世界法则')).toBeInTheDocument()
    expect(screen.queryByText('艾拉')).not.toBeInTheDocument()
  })

  it('shows a no-results hint when search matches nothing', async () => {
    const user = userEvent.setup()
    render(<ResourceDirectory sections={buildSections()} activeId={null} onSelect={() => {}} />)

    await user.type(screen.getByLabelText('搜索'), '不存在的内容')
    expect(screen.getByText('无匹配结果')).toBeInTheDocument()
  })

  it('supports controlled query', async () => {
    const user = userEvent.setup()
    const onQueryChange = vi.fn()
    render(<ResourceDirectory sections={buildSections()} activeId={null} onSelect={() => {}} query="艾拉" onQueryChange={onQueryChange} />)

    expect(screen.getByText('艾拉')).toBeInTheDocument()
    expect(screen.queryByText('凯尔')).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('搜索'), 'x')
    expect(onQueryChange).toHaveBeenCalled()
  })

  it('invokes section onCreate and disables it while saving', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn()
    const sections = buildSections()
    sections[0].onCreate = onCreate
    sections[0].createLabel = '新建角色'
    const { rerender } = render(<ResourceDirectory sections={sections} activeId={null} onSelect={() => {}} />)

    await user.click(screen.getByLabelText('新建角色'))
    expect(onCreate).toHaveBeenCalledTimes(1)

    rerender(<ResourceDirectory sections={sections} activeId={null} onSelect={() => {}} saving />)
    expect(screen.getByLabelText('新建角色')).toBeDisabled()
  })

  it('toggles section collapse via the chevron button', async () => {
    const user = userEvent.setup()
    render(<ResourceDirectory sections={buildSections()} activeId={null} onSelect={() => {}} />)

    await user.click(screen.getByLabelText('折叠角色'))
    expect(screen.queryByText('艾拉')).not.toBeInTheDocument()
    await user.click(screen.getByLabelText('展开角色'))
    expect(screen.getByText('艾拉')).toBeInTheDocument()
  })

  it('renders pinned entries above the groups and selects them', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(
      <ResourceDirectory
        sections={buildSections()}
        activeId="__agent__"
        onSelect={onSelect}
        pinnedEntries={[{ id: '__agent__', label: '配置管理 Agent', icon: Bot }]}
      />,
    )

    const pinned = screen.getByText('配置管理 Agent').closest('button')
    expect(pinned).toHaveClass('is-active')
    await user.click(pinned!)
    expect(onSelect).toHaveBeenCalledWith('__agent__')
  })

  it('keeps non-empty sections ahead of empty ones when emptySectionsLast is set', () => {
    const { container } = render(<ResourceDirectory sections={buildSections()} activeId={null} onSelect={() => {}} emptySectionsLast />)

    const labels = Array.from(container.querySelectorAll('section span.font-medium')).map((node) => node.textContent)
    expect(labels).toEqual(['角色', '规则', '地点'])
  })
})
