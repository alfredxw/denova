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
          director_plan: {
            story_id: 'st_1',
            branch_id: 'main',
            docs: {
              mainline: '# 大方向',
              current_event: '# 当前事件',
              next_branches: '# 最近分支',
            },
            visible_docs: {
              mainline: '青云逆袭主线',
              current_event: '外门比拼前夜，制造排名压力。',
            },
            metadata: {
              version: 1,
              story_id: 'st_1',
              branch_id: 'main',
              revision: 'rev-1',
              branch_planning_turns: 5,
              updated_at: '2026-05-17T00:00:00Z',
              docs: {
                mainline: { path: '/tmp/mainline.md', bytes: 10, hash: 'h1' },
                current_event: { path: '/tmp/current-event.md', bytes: 10, hash: 'h2' },
                next_branches: { path: '/tmp/next-branches.md', bytes: 10, hash: 'h3' },
              },
              last_run: { status: 'ready', summary: '已更新近期规划' },
            },
          },
          current_turn: {
            id: 'turn-2',
            parent_id: null,
            branch_id: 'main',
            ts: '2026-05-17T00:00:00Z',
            user: '我强行闯入藏书阁',
            narrative: '守阁长老拦在门前。',
            rule_resolution: {
              id: 'rr_1',
              created_at: '2026-05-17T00:00:00Z',
              accepted_brief: {
                user_action: '强行闯入藏书阁',
                intent: '冒险',
                turn_goal: '让错误选择产生明确代价',
                pressure: '守阁长老正在靠近',
                event_intents: ['冲突升级'],
                cost_policy: '失败会损失体力并暴露行踪',
                state_expectation: '体力下降，学院警戒升高',
                continuity_notes: '守阁长老不能被写成临时消失',
                rule_checks: [{ id: 'check_1', label: '潜入检定', kind: 'skill', dice: '1d20', difficulty: 18 }],
              },
              rule_results: [{
                id: 'check_1',
                label: '潜入检定',
                kind: 'skill',
                dice: '1d20',
                rolls: [4],
                roll_total: 4,
                modifier: 2,
                difficulty: 18,
                total: 6,
                outcome: 'failure',
                constraints: ['潜入检定失败，总值 6 / 难度 18。'],
              }],
              state_ops_preview: [{ op: 'inc', path: 'resources.hp', value: -10 }],
              terminal_candidate: { type: 'bad_end', reason: '强闯失败导致主线中断', check_id: 'check_1' },
              rule_constraints: ['潜入检定失败，总值 6 / 难度 18。'],
            },
            terminal_outcome: {
              terminal: true,
              type: 'bad_end',
              reason: '强闯失败导致主线中断',
              final_narrative_summary: '主角被逐出学院。',
              restart_suggestions: ['从上一安全回合创建新分支'],
            },
          },
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
    expect(screen.getByText('可选择')).toBeInTheDocument()
    expect(screen.getAllByText('地窖门').length).toBeGreaterThan(0)
    expect(screen.getByText('可能惊动柜台后的影子')).toBeInTheDocument()
    expect(screen.getByText('物品与资源')).toBeInTheDocument()
    expect(screen.getAllByText('火把').length).toBeGreaterThan(0)
    expect(screen.getByText('资源')).toBeInTheDocument()
    expect(screen.getAllByText('2').length).toBeGreaterThan(0)
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
    expect(screen.getByText('导演编排')).toBeInTheDocument()
    expect(screen.getByText('青云逆袭主线')).toBeInTheDocument()
    expect(screen.getByText('外门比拼前夜，制造排名压力。')).toBeInTheDocument()
    expect(screen.getByText('已更新近期规划')).toBeInTheDocument()
    expect(screen.getByText('规则审计')).toBeInTheDocument()
    expect(screen.getByText('本回合简报')).toBeInTheDocument()
    expect(screen.getByText('强行闯入藏书阁')).toBeInTheDocument()
    expect(screen.getByText('潜入检定')).toBeInTheDocument()
    expect(screen.getByText('failure')).toBeInTheDocument()
    expect(screen.getByText('终局候选')).toBeInTheDocument()
    expect(screen.getAllByText('强闯失败导致主线中断').length).toBeGreaterThan(0)
    expect(screen.getByText('重开建议')).toBeInTheDocument()
    expect(screen.getByText('从上一安全回合创建新分支')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent(/current_goal|last_seen_at|relationship_score|from_event/)
  })
})
