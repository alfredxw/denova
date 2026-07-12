import { useState } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ActorStateExplorer } from './ActorStateExplorer'
import type { ExplorerProps } from './types'

describe('ActorStateExplorer', () => {
	it('uses normalized names as IDs and rejects duplicate names within one template', async () => {
		const onValidityChange = vi.fn()
		render(
			<ActorStateExplorer
				value={{
					templates: [{
						id: 'protagonist',
						name: '主角状态',
						fields: [
							{ name: 'Ａ', type: 'string' },
							{ name: ' a ', type: 'string' },
						],
					}],
					initial_actors: [],
					trait_pools: [],
				}}
				onChange={vi.fn()}
				onValidityChange={onValidityChange}
			/>,
		)

		await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(false))
		expect(screen.queryByText(/^路径$|^Path$/)).not.toBeInTheDocument()
	})

  it('uses compact standalone sizing and exposes the state navigator as a tree', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <ActorStateExplorer
        value={{
          templates: [{
            id: 'protagonist',
            name: '主角状态',
            fields: [{ id: 'health', name: '身体状态', path: 'current.health', type: 'string', visibility: 'visible' }],
          }],
          initial_actors: [],
          trait_pools: [],
        }}
        onChange={vi.fn()}
        onValidityChange={vi.fn()}
      />,
    )

    expect(container.querySelector('.actor-state-explorer')).toHaveClass('min-h-[320px]')
    expect(container.querySelector('.actor-state-explorer')).not.toHaveClass('min-h-[540px]')
    expect(screen.getByRole('tree', { name: /状态结构|State Structure/ })).toHaveAttribute('aria-orientation', 'vertical')

    const templatesGroup = screen.getByRole('treeitem', { name: /状态表模板|State Table Templates/ })
    expect(templatesGroup).toHaveAttribute('aria-expanded', 'true')
    expect(templatesGroup).toHaveAttribute('aria-level', '1')

    const templateItem = screen.getByRole('treeitem', { name: '主角状态' })
    await waitFor(() => expect(templateItem).toHaveAttribute('aria-selected', 'true'))
    expect(templateItem).toHaveAttribute('aria-expanded', 'true')
    expect(templateItem).toHaveAttribute('aria-level', '2')
    expect(templateItem).toHaveAttribute('tabindex', '0')
    expect(templateItem.querySelector(':scope > div')?.className).not.toContain('inset_3px_0_0')
    expect(container.querySelector('.state-tree-branch')).toBeInTheDocument()
    const fieldItem = screen.getByRole('treeitem', { name: '身体状态' })
    expect(fieldItem).toHaveAttribute('aria-level', '3')

    templateItem.focus()
    await user.keyboard('{ArrowDown}')
    expect(fieldItem).toHaveAttribute('aria-selected', 'true')
    expect(fieldItem).toHaveFocus()

    const addTemplate = screen.getByRole('button', { name: /新增状态表模板|Add State Table Template/ })
    expect(addTemplate).toHaveClass('size-6', 'group-focus-within:opacity-100', 'focus-visible:opacity-100', '[@media(pointer:coarse)]:opacity-100')
    const collapseButton = screen.getAllByRole('button', { name: /^(折叠|Collapse)$/ })[0]
    expect(collapseButton).toHaveClass('size-6')
    expect(collapseButton).toHaveAttribute('aria-expanded', 'true')
  })

  it('uses a dismissible structure layer in a narrow editor pane', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <ActorStateExplorer
        layout="attached"
        value={{
          templates: [{
            id: 'protagonist',
            name: '主角状态',
            fields: [{ id: 'health', name: '身体状态', path: 'current.health', type: 'string', visibility: 'visible' }],
          }],
          initial_actors: [],
          trait_pools: [],
        }}
        onChange={vi.fn()}
        onValidityChange={vi.fn()}
      />,
    )

    const navigation = container.querySelector('.actor-state-navigation')
    const layout = container.querySelector('.actor-state-explorer-layout')
    expect(screen.getByTestId('actor-state-tree-scroll')).toHaveClass('actor-state-tree-scroll', 'overflow-hidden')
    expect(layout).toHaveClass('grid-rows-[minmax(0,1fr)]', 'overflow-hidden')
    expect(navigation).toHaveClass('h-full', 'min-h-0', 'overflow-hidden')
    expect(navigation).toHaveAttribute('data-open', 'false')

    await user.click(screen.getByRole('button', { name: '打开状态结构' }))
    expect(navigation).toHaveAttribute('data-open', 'true')

    await user.click(screen.getByRole('button', { name: /主角状态/ }))
    expect(navigation).toHaveAttribute('data-open', 'false')
    expect(screen.getByDisplayValue('主角状态')).toBeInTheDocument()
  })

  it('keeps template ID editing focused and updates linked actors atomically', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <StatefulExplorer
        initialValue={{
          templates: [
            { id: 'primary', name: 'Primary', fields: [] },
            { id: 'secondary', name: 'Secondary', fields: [] },
          ],
          initial_actors: [{ id: 'support', name: 'Support', template_id: 'secondary' }],
          trait_pools: [],
        }}
        onChange={onChange}
      />,
    )

    const secondaryItem = screen.getByRole('treeitem', { name: 'Secondary' })
    await user.click(within(secondaryItem).getByTitle(/^Secondary/))
    const idInput = await screen.findByDisplayValue('secondary')
    await user.click(idInput)
    fireEvent.change(idInput, { target: { value: 'renamed' } })

    expect(screen.getByDisplayValue('renamed')).toHaveFocus()
    expect(screen.getByRole('treeitem', { name: 'Secondary' })).toHaveAttribute('aria-selected', 'true')
    expect(onChange.mock.lastCall?.[0]).toMatchObject({
      templates: [{ id: 'primary' }, { id: 'renamed' }],
      initial_actors: [{ id: 'support', template_id: 'renamed' }],
    })
  })

  it('keeps an actor selected while its ID changes', async () => {
    const user = userEvent.setup()
    render(
      <StatefulExplorer
        initialValue={{
          templates: [{ id: 'primary', name: 'Primary', fields: [] }],
          initial_actors: [
            { id: 'actor-a', name: 'Actor A', template_id: 'primary' },
            { id: 'actor-b', name: 'Actor B', template_id: 'primary' },
          ],
          trait_pools: [],
        }}
      />,
    )

    const actorGroup = screen.getByRole('treeitem', { name: /初始状态对象|Initial State Objects/ })
    expect(actorGroup).toHaveAttribute('aria-expanded', 'true')
    const actorItem = screen.getByRole('treeitem', { name: 'Actor B' })
    await user.click(within(actorItem).getByTitle(/^Actor B/))
    fireEvent.change(await screen.findByDisplayValue('actor-b'), { target: { value: 'actor-renamed' } })
    expect(screen.getByDisplayValue('actor-renamed')).toBeInTheDocument()
    expect(screen.getByRole('treeitem', { name: 'Actor B' })).toHaveAttribute('aria-selected', 'true')
  })

  it('keeps a trait pool and trait selected while IDs change and cascades template rules', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <StatefulExplorer
        initialValue={{
          templates: [{ id: 'primary', name: 'Primary', fields: [], trait_rules: [{ pool_id: 'pool-b', draw_count: 1 }] }],
          initial_actors: [],
          trait_pools: [
            { id: 'pool-a', name: 'Pool A', traits: [] },
            { id: 'pool-b', name: 'Pool B', traits: [{ id: 'trait-b', name: 'Trait B', weight: 1, visibility: 'visible' }] },
          ],
        }}
        onChange={onChange}
      />,
    )

    let poolItem = screen.getByRole('treeitem', { name: 'Pool B' })
    await user.click(within(poolItem).getByTitle(/^Pool B/))
    fireEvent.change(await screen.findByDisplayValue('pool-b'), { target: { value: 'pool-renamed' } })
    expect(screen.getByDisplayValue('pool-renamed')).toBeInTheDocument()
    poolItem = screen.getByRole('treeitem', { name: 'Pool B' })
    expect(poolItem).toHaveAttribute('aria-selected', 'true')
    expect(onChange.mock.lastCall?.[0]).toMatchObject({
      templates: [{ id: 'primary', trait_rules: [{ pool_id: 'pool-renamed', draw_count: 1 }] }],
    })

    const traitItem = screen.getByRole('treeitem', { name: 'Trait B' })
    expect(within(traitItem).getByText(/可见|Visible/)).toBeInTheDocument()
    await user.click(within(traitItem).getByTitle(/^Trait B/))
    fireEvent.change(await screen.findByDisplayValue('trait-b'), { target: { value: 'trait-renamed' } })
    expect(screen.getByDisplayValue('trait-renamed')).toBeInTheDocument()
    expect(screen.getByRole('treeitem', { name: 'Trait B' })).toHaveAttribute('aria-selected', 'true')
  })
})

function StatefulExplorer({
  initialValue,
  onChange,
}: {
  initialValue: ExplorerProps['value']
  onChange?: (value: ExplorerProps['value']) => void
}) {
  const [value, setValue] = useState(initialValue)
  return (
    <ActorStateExplorer
      value={value}
      onChange={(nextValue) => {
        setValue(nextValue)
        onChange?.(nextValue)
      }}
      onValidityChange={vi.fn()}
    />
  )
}
