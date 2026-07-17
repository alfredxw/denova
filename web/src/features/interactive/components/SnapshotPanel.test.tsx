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
          director_plan_status: {
            story_id: 'st_1',
            branch_id: 'main',
            status: 'ready',
            summary: '已更新近期规划',
            updated_at: '2026-05-17T00:00:00Z',
            planned_docs: 1,
            completed_docs: 1,
            doc_bytes: 30,
            visible_bytes: 20,
            start_ready: true,
            blocking: false,
            revision: 'rev-1',
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
              request: {
                action: '强行闯入藏书阁',
                intent: '冒险',
                challenge: '潜入检定',
                cost: '失败会损失体力并暴露行踪',
                state: '守阁长老正在靠近',
                adjudication: {
                  reason: '强闯会改变守阁长老的警戒状态。',
                  stakes: '失败会暴露行踪。',
                  difficulty_reason: '守阁长老靠近，难度提高。',
                  roll_mode_reason: '熟悉地形但没有掩护，正常投骰。',
                  state_refs: [{ actor_id: 'protagonist', field_id: '当前生命' }],
                },
                rule: { template: 'dice_check', template_id: 'stealth-lock', label: '潜行与开锁', failure_policy: 'blocked', roll_mode: 'normal' },
                bonuses: [{ kind: 'environment', reason: '熟悉地形', value: 2 }],
                difficulty: 'hard',
                outcomes: {
                  critical_success: { result: '无声潜入。' },
                  success: { result: '成功潜入。' },
                  failure: { result: '强闯失败导致主线中断', state_changes: [{ actor_id: 'protagonist', field_id: '当前生命', change: -10, reason: '被禁制反震' }] },
                  critical_failure: { result: '被当场抓住。' },
                },
              },
              result: {
                id: 'check_1',
                label: '潜入检定',
                kind: 'skill',
                dice: '1d20',
                roll_mode: 'normal',
                rolls: [4],
                roll_total: 4,
                kept_roll: 4,
                base_target: 15,
                bonus_total: 2,
                bonus_details: [{ kind: 'environment', reason: '熟悉地形', value: 2 }],
                modifier: 2,
                difficulty: 18,
                target: 18,
                total: 6,
                outcome: 'failure',
                result: '强闯失败导致主线中断',
                state_changes: [{ actor_id: 'protagonist', field_id: '当前生命', change: -10, reason: '被禁制反震' }],
                constraints: ['潜入检定失败，总值 6 / 难度 18。'],
              },
              state_consumption: {
                status: 'partial',
                mode: 'hybrid_auto',
                applied_actor_ops: [{ op: 'set', actor_id: 'protagonist', field_id: '当前生命', value: 0, reason: '被禁制反震', source_kind: 'rule_resolution', source_id: 'rr_1' }],
                warnings: [{ actor_id: 'protagonist', field_id: '中毒', reason: '字段不在状态系统中' }],
              },
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
    expect(screen.getAllByText('ready').length).toBeGreaterThan(0)
    expect(screen.getByText('1/1')).toBeInTheDocument()
    expect(screen.getByText('已更新近期规划')).toBeInTheDocument()
    expect(screen.queryByText('青云逆袭主线')).not.toBeInTheDocument()
    expect(screen.queryByText('外门比拼前夜，制造排名压力。')).not.toBeInTheDocument()
    expect(screen.getByText('规则审计')).toBeInTheDocument()
    expect(screen.getByText('本次检定')).toBeInTheDocument()
    expect(screen.getByText('投前裁定')).toBeInTheDocument()
    expect(screen.getByText('强闯会改变守阁长老的警戒状态。')).toBeInTheDocument()
    expect(screen.getByText('基础目标')).toBeInTheDocument()
    expect(screen.getByText('状态消费')).toBeInTheDocument()
    expect(screen.getByText('hybrid_auto')).toBeInTheDocument()
    expect(screen.getByText('字段不在状态系统中')).toBeInTheDocument()
    expect(screen.getByText('强行闯入藏书阁')).toBeInTheDocument()
    expect(screen.getAllByText('潜入检定').length).toBeGreaterThan(0)
    expect(screen.getAllByText('failure').length).toBeGreaterThan(0)
    expect(screen.getByText('终局候选')).toBeInTheDocument()
    expect(screen.getAllByText('强闯失败导致主线中断').length).toBeGreaterThan(0)
    expect(screen.getByText('重开建议')).toBeInTheDocument()
    expect(screen.getByText('从上一安全回合创建新分支')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent(/current_goal|last_seen_at|relationship_score|from_event/)
  })
})
