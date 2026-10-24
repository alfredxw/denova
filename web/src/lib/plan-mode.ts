const MAX_APPROVED_PLAN_CHARS = 16_000

export function formatApprovedPlanExecutionMessage(planContent: string, originalRequest?: string) {
  const plan = truncatePlanContext(planContent)
  const request = originalRequest?.trim()
  return [
    '[Plan approved]',
    request ? `Исходный запрос и дополнения пользователя / Original request and clarifications:\n${request}` : '',
    `Утверждённый план / Approved plan:\n<approved_plan>\n${plan}\n</approved_plan>`,
    'Пожалуйста, строго следуйте утверждённому плану. Если обнаружится конфликт с реальным кодом, сначала опишите его и запросите подтверждение, не расширяя область изменений самостоятельно.',
  ].filter(Boolean).join('\n\n')
}

export function formatPlanDiscussionMessage(planContent: string) {
  return `Я хочу продолжить обсуждение этого плана / I want to discuss this plan:\n<proposed_plan>\n${truncatePlanContext(planContent)}\n</proposed_plan>\n\nПункты для изменения / Adjustments:`
}

export function planDisplayContent(content: string) {
  return content.trim()
}

function truncatePlanContext(content: string) {
  const chars = Array.from(content.trim())
  if (chars.length <= MAX_APPROVED_PLAN_CHARS) return content.trim()
  return `${chars.slice(0, MAX_APPROVED_PLAN_CHARS).join('').trimEnd()}\n\n[truncated to ${MAX_APPROVED_PLAN_CHARS} chars]`
}
