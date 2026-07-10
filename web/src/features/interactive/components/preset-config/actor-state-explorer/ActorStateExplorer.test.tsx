import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ActorStateExplorer } from './ActorStateExplorer'

describe('ActorStateExplorer', () => {
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
          initial_state_ops: [],
          opening_enabled: true,
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
})
