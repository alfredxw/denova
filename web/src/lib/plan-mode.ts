const MAX_APPROVED_PLAN_CHARS = 16_000

export function formatApprovedPlanExecutionMessage(planContent: string, originalRequest?: string) {
  const plan = truncatePlanContext(planContent)
  const request = originalRequest?.trim()
  return [
    '[Plan approved]',
    request ? `原始请求与用户补充：\n${request}` : '',
    `已批准计划：\n<approved_plan>\n${plan}\n</approved_plan>`,
    '请严格按已批准计划执行。若执行中发现计划与真实代码冲突，请先说明冲突并请求确认，不要自行扩大范围。',
  ].filter(Boolean).join('\n\n')
}

export function formatPlanDiscussionMessage(planContent: string) {
  return `我想继续讨论这份计划：\n<proposed_plan>\n${truncatePlanContext(planContent)}\n</proposed_plan>\n\n调整点：`
}

export function planDisplayContent(content: string) {
  return content.trim()
}

function truncatePlanContext(content: string) {
  const chars = Array.from(content.trim())
  if (chars.length <= MAX_APPROVED_PLAN_CHARS) return content.trim()
  return `${chars.slice(0, MAX_APPROVED_PLAN_CHARS).join('').trimEnd()}\n\n[truncated to ${MAX_APPROVED_PLAN_CHARS} chars]`
}
