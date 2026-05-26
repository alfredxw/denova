import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SnapshotPanel } from './SnapshotPanel'

describe('SnapshotPanel', () => {
  it('renders character states and key events as readable fields', () => {
    render(
      <SnapshotPanel
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          turns: [],
          state: {
            on_stage: ['林川'],
            characters: {
              林川: {
                location: '黄泉酒馆',
                mood: '警惕',
                hp: 80,
                items: ['火把', '铜钥匙'],
                current_goal: '找到柜台后的密门',
                last_seen_at: '午夜',
                relationship_score: 12,
              },
            },
            events: [
              {
                type: '线索',
                title: '墙上的新线索',
                description: '火光照出墙缝里的旧字。',
                time: '午夜',
                from_event: 'ev_1',
              },
              '酒馆门自行关上',
            ],
          },
        }}
      />,
    )

    expect(screen.getAllByText('林川').length).toBeGreaterThan(0)
    expect(screen.getByText('位置')).toBeInTheDocument()
    expect(screen.getByText('黄泉酒馆')).toBeInTheDocument()
    expect(screen.getByText('情绪')).toBeInTheDocument()
    expect(screen.getByText('警惕')).toBeInTheDocument()
    expect(screen.getByText('体力')).toBeInTheDocument()
    expect(screen.getByText('80')).toBeInTheDocument()
    expect(screen.getByText('火把')).toBeInTheDocument()
    expect(screen.getByText('铜钥匙')).toBeInTheDocument()
    expect(screen.getByText('当前目标')).toBeInTheDocument()
    expect(screen.getByText('找到柜台后的密门')).toBeInTheDocument()
    expect(screen.getByText('最后出现')).toBeInTheDocument()
    expect(screen.getByText('关系值')).toBeInTheDocument()
    expect(screen.getByText('墙上的新线索')).toBeInTheDocument()
    expect(screen.getByText('火光照出墙缝里的旧字。')).toBeInTheDocument()
    expect(screen.getByText('线索')).toBeInTheDocument()
    expect(screen.getByText('来源事件')).toBeInTheDocument()
    expect(screen.getByText('酒馆门自行关上')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent(/current_goal|last_seen_at|relationship_score|from_event/)
  })
})
