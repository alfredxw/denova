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
            scene: {
              danger_level: '升高',
              atmosphere: '酒馆里只剩火把的噼啪声',
              interactive_objects: ['柜台', '地窖门'],
            },
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
            inventory: {
              林川: ['火把', '铜钥匙'],
            },
            resources: {
              torch_fuel: 2,
            },
            world_flags: ['黄泉酒馆会回应火光'],
            rules: ['午夜后只进不出'],
            threads: [{ title: '柜台后的影子', status: '未解决' }],
            action_space: [{ target: '地窖门', risk: '可能惊动柜台后的影子' }],
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
    expect(screen.getAllByText('火把').length).toBeGreaterThan(0)
    expect(screen.getAllByText('铜钥匙').length).toBeGreaterThan(0)
    expect(screen.getByText('当前目标')).toBeInTheDocument()
    expect(screen.getByText('找到柜台后的密门')).toBeInTheDocument()
    expect(screen.getByText('最后出现')).toBeInTheDocument()
    expect(screen.getByText('关系值')).toBeInTheDocument()
    expect(screen.getByText('可行动空间')).toBeInTheDocument()
    expect(screen.getAllByText('地窖门').length).toBeGreaterThan(0)
    expect(screen.getByText('可能惊动柜台后的影子')).toBeInTheDocument()
    expect(screen.getByText('物品与资源')).toBeInTheDocument()
    expect(screen.getAllByText('火把').length).toBeGreaterThan(0)
    expect(screen.getByText('资源')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('规则与暗线')).toBeInTheDocument()
    expect(screen.getByText('黄泉酒馆会回应火光')).toBeInTheDocument()
    expect(screen.getByText('午夜后只进不出')).toBeInTheDocument()
    expect(screen.getByText('柜台后的影子')).toBeInTheDocument()
    expect(screen.getByText('升高')).toBeInTheDocument()
    expect(screen.getByText('墙上的新线索')).toBeInTheDocument()
    expect(screen.getByText('火光照出墙缝里的旧字。')).toBeInTheDocument()
    expect(screen.getByText('线索')).toBeInTheDocument()
    expect(screen.getByText('来源事件')).toBeInTheDocument()
    expect(screen.getByText('酒馆门自行关上')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent(/current_goal|last_seen_at|relationship_score|from_event/)
  })
})
