import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Snapshot, TurnEvent } from '../../types'
import { OverviewView } from './OverviewView'

const turn: TurnEvent = {
  id: 'turn-1',
  parent_id: null,
  branch_id: 'main',
  ts: '2026-08-31T00:00:00Z',
  user: '进入车站',
  narrative: '沈凝走进旧车站。',
  state_delta: {
    actor_ops: [{ op: 'set', actor_id: 'hero', field_id: 'health', value: 7 }],
  },
}

const snapshot: Snapshot = {
  story_id: 'story-1',
  branch_id: 'main',
  turns: [turn],
  current_turn: turn,
  state: {
    actors: {
      hero: { name: '沈凝', role: 'protagonist', state: { health: 7 } },
    },
    scene: '旧车站',
  },
}

describe('OverviewView', () => {
  it('keeps state compact and opens the shared full-state view', () => {
    render(<OverviewView snapshot={snapshot} planningEnabled={false} />)

    expect(screen.getByText('本回合 1 项变化')).toBeInTheDocument()
    expect(screen.getByText('1 位角色 · 1 项世界状态')).toBeInTheDocument()
    expect(screen.queryByLabelText('选择状态展示方式')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /完整状态/ }))
    expect(screen.getByRole('dialog', { name: '故事状态' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '沈凝' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '世界状态' })).toBeInTheDocument()
  })

  it('keeps an empty story compact while retaining the full-state entry', () => {
    render(<OverviewView snapshot={null} planningEnabled={false} />)

    expect(screen.getByText('本回合还没有提交状态变化。')).toBeInTheDocument()
    expect(screen.getByText('0 位角色 · 0 项世界状态')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /完整状态/ }))
    expect(screen.getByText('当前分支暂无结构化状态')).toBeInTheDocument()
  })
})
