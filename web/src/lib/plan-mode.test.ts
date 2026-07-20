import { describe, expect, it } from 'vitest'
import {
  formatApprovedPlanExecutionMessage,
  parsePlanQuestionSet,
  planDisplayContent,
  recommendedAnswerSet,
} from './plan-mode'

describe('plan-mode helpers', () => {
  it('解析问题卡 JSON 并生成推荐答案', () => {
    const parsed = parsePlanQuestionSet(JSON.stringify({
      questions: [{
        id: 'scope',
        type: 'multi',
        question: '实现范围？',
        options: [
          { id: 'chat', label: 'IDE Chat', recommended: true },
          { id: 'interactive', label: '互动模式', recommended: true },
        ],
      }],
    }))

    expect(parsed?.questions[0]).toMatchObject({ id: 'scope', type: 'multi', question: '实现范围？' })
    expect(recommendedAnswerSet(parsed!)).toEqual({ scope: ['chat', 'interactive'] })
  })

  it('非法问题 JSON 返回 null', () => {
    expect(parsePlanQuestionSet('{broken')).toBeNull()
    expect(parsePlanQuestionSet('{"questions":[]}')).toBeNull()
  })

  it('保留多个问题，由前端逐题确认后统一提交', () => {
    const parsed = parsePlanQuestionSet(JSON.stringify({
      questions: [
        {
          id: 'scope',
          question: '先确认范围？',
          options: [
            { id: 'chat', label: '仅写作 Chat', recommended: true },
            { id: 'all', label: '全部模式' },
          ],
        },
        {
          id: 'style',
          question: '再确认交互样式？',
          options: [
            { id: 'one', label: '逐题' },
            { id: 'batch', label: '批量' },
          ],
        },
      ],
    }))

    expect(parsed?.questions).toHaveLength(2)
    expect(parsed?.questions[0].id).toBe('scope')
    expect(parsed?.questions[1].id).toBe('style')
  })

  it('完整展示计划卡但限制确认执行上下文长度', () => {
    const longPlan = '计划'.repeat(9000)
    const display = planDisplayContent(longPlan)
    const execution = formatApprovedPlanExecutionMessage(longPlan, '原始需求')

    expect(display).toBe(longPlan)
    expect(execution).toContain('<approved_plan>')
    expect(execution.length).toBeLessThan(longPlan.length + 200)
  })
})
