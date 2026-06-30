import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TurnNavigator } from './TurnNavigator'

describe('TurnNavigator', () => {
  it('renders one bar per turn with controlled previews and active state', () => {
    render(
      <TurnNavigator
        items={[
          { anchorId: 'turn-1', user: '推开酒馆的门', narrative: '门后传来低沉的风声。' },
          { anchorId: 'turn-2', user: '走向壁炉', narrative: '炉火映出墙上的旧徽记。' },
        ]}
        activeAnchorId="turn-2"
        onSelect={vi.fn()}
      />,
    )

    expect(screen.getByRole('complementary', { name: '剧情轮次导航' })).toBeInTheDocument()
    expect(screen.getAllByRole('button')).toHaveLength(2)
    fireEvent.mouseEnter(screen.getByRole('button', { name: '跳转到第 1 轮' }))
    expect(screen.getByText('推开酒馆的门')).toBeInTheDocument()
    expect(screen.getByText('门后传来低沉的风声。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '跳转到第 2 轮' })).toHaveAttribute('aria-current', 'true')
    expect(screen.getByRole('button', { name: '跳转到第 1 轮' })).not.toHaveAttribute('title')
  })

  it('selects turns by click and remains reachable by keyboard focus', async () => {
    const user = userEvent.setup()
    const handleSelect = vi.fn()
    render(
      <TurnNavigator
        items={[
          { anchorId: 'turn-1', user: '第一轮', narrative: '第一段剧情。' },
          { anchorId: 'turn-2', user: '第二轮', narrative: '第二段剧情。' },
        ]}
        activeAnchorId="turn-1"
        onSelect={handleSelect}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '跳转到第 2 轮' }))
    expect(handleSelect).toHaveBeenCalledWith('turn-2')

    await user.tab()
    expect(screen.getByRole('button', { name: '跳转到第 1 轮' })).toHaveFocus()
  })
})
