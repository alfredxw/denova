import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { PresetTabsList, reorderPresetTabsListItems } from './PresetTabsList'

interface Item {
  id: string
  title: string
  subtitle: string
}

const items: Item[] = [
  { id: 'enemy', title: '敌人/怪物状态', subtitle: '11 个字段' },
  { id: 'protagonist', title: '主角状态', subtitle: '15 个字段' },
  { id: 'important', title: '重要角色状态', subtitle: '13 个字段' },
]

function renderPresetTabsList(overrides: Partial<{
  activeId: string
  items: Item[]
  layout: 'panel' | 'rail'
  onAdd: () => void
  onActiveIdChange: (id: string) => void
  onItemsChange: (items: Item[]) => void
}> = {}) {
  const onAdd = overrides.onAdd || vi.fn()
  const onActiveIdChange = overrides.onActiveIdChange || vi.fn()
  const onItemsChange = overrides.onItemsChange || vi.fn()
  const listItems = overrides.items ?? items
  function Harness() {
    const [activeId, setActiveId] = useState(overrides.activeId ?? 'enemy')
    return (
      <PresetTabsList
        items={listItems}
        activeId={activeId}
        getId={(item) => item.id}
        getTitle={(item) => item.title}
        getSubtitle={(item) => item.subtitle}
        addLabel="新增状态表模板"
        emptyLabel="状态表模板"
        layout={overrides.layout}
        onAdd={onAdd}
        onActiveIdChange={(id) => {
          setActiveId(id)
          onActiveIdChange(id)
        }}
        onItemsChange={onItemsChange}
      />
    )
  }
  const view = render(<Harness />)
  return { ...view, onAdd, onActiveIdChange, onItemsChange }
}

describe('PresetTabsList', () => {
  it('switches tabs from title, subtitle, and trigger surface clicks', async () => {
    const user = userEvent.setup()
    const { onActiveIdChange } = renderPresetTabsList()

    await user.click(screen.getByText('主角状态'))
    expect(onActiveIdChange).toHaveBeenLastCalledWith('protagonist')

    await user.click(screen.getByText('13 个字段'))
    expect(onActiveIdChange).toHaveBeenLastCalledWith('important')

    await user.click(screen.getByTestId('preset-tabs-list-trigger-enemy'))
    expect(onActiveIdChange).toHaveBeenLastCalledWith('enemy')
  })

  it('keeps the drag handle separate from tab selection', async () => {
    const user = userEvent.setup()
    const { onActiveIdChange } = renderPresetTabsList()

    const dragHandle = screen.getByRole('button', { name: '拖动 主角状态' })
    expect(dragHandle).toHaveClass('size-5')
    await user.click(dragHandle)

    expect(onActiveIdChange).not.toHaveBeenCalled()
    expect(screen.getByTestId('preset-tabs-list-item-protagonist').className).not.toContain('inset_3px_0_0')
  })

  it('computes the drag-end order without changing the active id', () => {
    const next = reorderPresetTabsListItems(items, items.map((item) => item.id), 'enemy', 'protagonist')

    expect(next).toEqual([items[1], items[0], items[2]])
    expect(next.map((item) => item.id)).toContain('enemy')
  })

  it('renders the empty state and add action', () => {
    const { onAdd } = renderPresetTabsList({ items: [] })

    expect(screen.getAllByText('状态表模板')).toHaveLength(2)
    fireEvent.click(screen.getByRole('button', { name: '新增状态表模板' }))
    expect(onAdd).toHaveBeenCalledTimes(1)
  })

  it('lets rail height follow its parent instead of the viewport', () => {
    const { container } = renderPresetTabsList({ layout: 'rail' })
    const rail = container.querySelector('aside')

    expect(rail).toHaveClass('h-full', 'min-h-0', 'rounded-[12px]')
    expect(rail?.className).not.toMatch(/sticky|100vh|max-h/)
  })
})
