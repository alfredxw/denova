import { describe, expect, it } from 'vitest'
import {
  formatApprovedPlanExecutionMessage,
  planDisplayContent,
} from './plan-mode'

describe('plan-mode helpers', () => {
  it('完整展示计划卡但限制确认执行上下文长度', () => {
    const longPlan = '计划'.repeat(9000)
    const display = planDisplayContent(longPlan)
    const execution = formatApprovedPlanExecutionMessage(longPlan, '原始需求')

    expect(display).toBe(longPlan)
    expect(execution).toContain('<approved_plan>')
    expect(execution.length).toBeLessThan(longPlan.length + 200)
  })
})
