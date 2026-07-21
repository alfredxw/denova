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

	it('rejects path separators in field names and explains the constraint', async () => {
		const user = userEvent.setup()
		const onValidityChange = vi.fn()
		render(
			<ActorStateExplorer
				value={{
					templates: [{
						id: 'protagonist',
						name: '主角状态',
						fields: [{ name: '当前精神/意志状态', type: 'string' }],
					}],
					initial_actors: [],
					trait_pools: [],
				}}
				onChange={vi.fn()}
				onValidityChange={onValidityChange}
			/>,
		)

		await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(false))
		const fieldItem = screen.getByRole('treeitem', { name: '当前精神/意志状态' })
		await user.click(within(fieldItem).getByTitle(/^当前精神\/意志状态/))
		const input = await screen.findByDisplayValue('当前精神/意志状态')
		expect(input).toHaveAttribute('aria-invalid', 'true')
		expect(screen.getByRole('alert')).toHaveTextContent(/路径分隔符.*\/|path separator.*\//i)
	})

  it('uses compact standalone sizing and exposes the state navigator as a tree', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <ActorStateExplorer
        value={{
          templates: [{
            id: 'protagonist',
            name: '主角状态',
            fields: [{ id: 'health', name: '身体状态', path: 'current.health', type: 'string', group: '状态' }],
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
    const fieldGroup = screen.getByRole('treeitem', { name: '状态' })
    expect(fieldGroup).toHaveAttribute('aria-expanded', 'true')
    expect(fieldGroup).toHaveAttribute('aria-level', '3')
    const fieldItem = screen.getByRole('treeitem', { name: '身体状态' })
    expect(fieldItem).toHaveAttribute('aria-level', '4')

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

  it('switches between field details immediately without waiting for a transition', async () => {
    const user = userEvent.setup()
    render(
      <ActorStateExplorer
        value={{
          templates: [{
            id: 'protagonist',
            name: 'Protagonist',
            fields: [
              { id: 'health', name: 'Health', type: 'number' },
              { id: 'mana', name: 'Mana', type: 'number' },
            ],
          }],
          initial_actors: [],
          trait_pools: [],
        }}
        onChange={vi.fn()}
        onValidityChange={vi.fn()}
      />,
    )

    const healthItem = screen.getByRole('treeitem', { name: 'Health' })
    await user.click(within(healthItem).getByTitle(/^Health/))
    expect(await screen.findByDisplayValue('Health')).toBeInTheDocument()

    const manaItem = screen.getByRole('treeitem', { name: 'Mana' })
    fireEvent.click(within(manaItem).getByTitle(/^Mana/))
    expect(screen.getByDisplayValue('Mana')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('Health')).not.toBeInTheDocument()
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
            fields: [{ id: 'health', name: '身体状态', path: 'current.health', type: 'string' }],
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

  it('edits field display group and display hint from the field detail pane', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <StatefulExplorer
        initialValue={{
          templates: [{
            id: 'protagonist',
            name: '主角状态',
            fields: [{ name: '当前处境', type: 'string' }],
          }],
          initial_actors: [],
          trait_pools: [],
        }}
        onChange={onChange}
      />,
    )

    const fieldItem = screen.getByRole('treeitem', { name: '当前处境' })
    await user.click(within(fieldItem).getByTitle(/^当前处境/))

    const groupInput = await screen.findByPlaceholderText(/留空自动分组|auto grouping/i)
    await user.type(groupInput, '战斗')
    expect(onChange.mock.lastCall?.[0]).toMatchObject({
      templates: [{ fields: [{ name: '当前处境', group: '战斗' }] }],
    })

    const displayLabel = screen.getByText(/^展示方式$|^Display$/)
    const displaySelect = displayLabel.parentElement?.querySelector('button[role="combobox"]')
    expect(displaySelect).not.toBeNull()
    await user.click(displaySelect as HTMLElement)
    await user.click(await screen.findByRole('option', { name: /段落块|Paragraph block/ }))
    expect(onChange.mock.lastCall?.[0]).toMatchObject({
      templates: [{ fields: [{ name: '当前处境', display: 'block' }] }],
    })
  })

  it('does not expose legacy field or trait visibility metadata in the tree and detail editors', async () => {
    const user = userEvent.setup()
    const legacyValue = {
      templates: [{
        id: 'protagonist',
        name: '主角状态',
        fields: [{ name: '当前处境', type: 'string', visibility: 'hidden' }],
      }],
      initial_actors: [],
      trait_pools: [{
        id: 'origin',
        name: '出身',
        traits: [{ id: 'wanderer', name: '游侠', weight: 1, visibility: 'spoiler' }],
      }],
    } as unknown as ExplorerProps['value']

    render(
      <ActorStateExplorer
        value={legacyValue}
        onChange={vi.fn()}
        onValidityChange={vi.fn()}
      />,
    )

    const fieldItem = screen.getByRole('treeitem', { name: '当前处境' })
    expect(within(fieldItem).queryByText(/^隐藏$|^Hidden$/)).not.toBeInTheDocument()
    await user.click(within(fieldItem).getByTitle(/^当前处境/))
    expect(screen.queryByText(/^可见性$|^Visibility$/)).not.toBeInTheDocument()
    expect(screen.queryByText(/^隐藏$|^Hidden$/)).not.toBeInTheDocument()

    const traitItem = screen.getByRole('treeitem', { name: '游侠' })
    expect(within(traitItem).queryByText(/^剧透$|^Spoiler$/)).not.toBeInTheDocument()
    await user.click(within(traitItem).getByTitle(/^游侠/))
    expect(screen.queryByText(/^可见性$|^Visibility$/)).not.toBeInTheDocument()
    expect(screen.queryByText(/^剧透$|^Spoiler$/)).not.toBeInTheDocument()
  })

  it('keeps section and renderer hints in the schema without exposing UI ordering controls', async () => {
    render(
      <StatefulExplorer
        initialValue={{
          templates: [{
            id: 'protagonist',
            name: '主角状态',
            fields: [
              { name: '称号', type: 'string', group: '身份' },
              { name: '声望', type: 'string', group: '人际' },
            ],
          }],
          initial_actors: [],
          trait_pools: [],
        }}
        onChange={() => undefined}
      />,
    )

    expect(screen.queryByRole('button', { name: /下移分组|Move group down/ })).not.toBeInTheDocument()
    expect(screen.queryByText(/^排序$|^Order$/)).not.toBeInTheDocument()
  })

  it('opens nested object defaults as JSON so panel values stay readable and editable', async () => {
    const user = userEvent.setup()
    const panelDefault = {
      力量: { 基础值: 10, 当前值: 12, 修正说明: '装备 +2' },
    }
    render(
      <ActorStateExplorer
        value={{
          templates: [{
            id: 'protagonist',
            name: '主角状态',
            fields: [{ name: '面板', type: 'object', default: panelDefault }],
          }],
          initial_actors: [],
          trait_pools: [],
        }}
        onChange={vi.fn()}
        onValidityChange={vi.fn()}
      />,
    )

    const panelItem = screen.getByRole('treeitem', { name: '面板' })
    await user.click(within(panelItem).getByTitle(/^面板/))

    const expectedJSON = JSON.stringify(panelDefault, null, 2)
    const jsonEditor = screen.getAllByRole('textbox').find((element) => (
      element.tagName === 'TEXTAREA' && (element as HTMLTextAreaElement).value === expectedJSON
    ))
    expect(jsonEditor).toBeInTheDocument()
    expect(screen.queryByDisplayValue('[object Object]')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /JSON/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /结构化|Structured/ })).toBeDisabled()
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
            { id: 'pool-b', name: 'Pool B', traits: [{ id: 'trait-b', name: 'Trait B', weight: 1 }] },
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
    expect(within(traitItem).queryByText(/可见|Visible/)).not.toBeInTheDocument()
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
