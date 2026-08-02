type PlanQuestionType = 'single' | 'multi'

interface PlanQuestionOption {
  id: string
  label: string
  description?: string
  recommended?: boolean
}

interface PlanQuestion {
  id: string
  type: PlanQuestionType
  question: string
  description?: string
  options: PlanQuestionOption[]
  allow_custom?: boolean
}

export interface PlanQuestionSet {
  questions: PlanQuestion[]
}

const MAX_APPROVED_PLAN_CHARS = 16_000

export function parsePlanQuestionSet(content: string): PlanQuestionSet | null {
  const raw = content.trim()
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    const source = Array.isArray(parsed) ? parsed : parsed?.questions
    if (!Array.isArray(source)) return null
    const questions = source.map(normalizeQuestion).filter((item): item is PlanQuestion => Boolean(item))
    return questions.length > 0 ? { questions } : null
  } catch {
    return null
  }
}

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

function normalizeQuestion(value: unknown, index: number): PlanQuestion | null {
  if (!value || typeof value !== 'object') return null
  const item = value as Record<string, unknown>
  const question = readString(item.question)
  if (!question) return null
  const id = readString(item.id) || `question_${index + 1}`
  const type = item.type === 'multi' ? 'multi' : 'single'
  const options = Array.isArray(item.options)
    ? item.options.map((option, optionIndex) => normalizeOption(option, optionIndex)).filter((option): option is PlanQuestionOption => Boolean(option))
    : []
  if (options.length === 0) return null
  return {
    id,
    type,
    question,
    description: readString(item.description),
    options,
    allow_custom: item.allow_custom !== false,
  }
}

function normalizeOption(value: unknown, index: number): PlanQuestionOption | null {
  if (!value || typeof value !== 'object') return null
  const item = value as Record<string, unknown>
  const label = readString(item.label)
  if (!label) return null
  return {
    id: readString(item.id) || `option_${index + 1}`,
    label,
    description: readString(item.description),
    recommended: item.recommended === true,
  }
}

function readString(value: unknown) {
  return typeof value === 'string' ? value.trim() : ''
}

function truncatePlanContext(content: string) {
  const chars = Array.from(content.trim())
  if (chars.length <= MAX_APPROVED_PLAN_CHARS) return content.trim()
  return `${chars.slice(0, MAX_APPROVED_PLAN_CHARS).join('').trimEnd()}\n\n[truncated to ${MAX_APPROVED_PLAN_CHARS} chars]`
}
