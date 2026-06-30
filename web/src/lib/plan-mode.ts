export type PlanQuestionType = 'single' | 'multi'

export interface PlanQuestionOption {
  id: string
  label: string
  description?: string
  recommended?: boolean
}

export interface PlanQuestion {
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

export interface PlanQuestionAnswer {
  questionId: string
  question: string
  selectedOptions: PlanQuestionOption[]
  customAnswer?: string
}

export const PLAN_CARD_DISPLAY_CHARS = 12_000
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

export function recommendedAnswerSet(questionSet: PlanQuestionSet): Record<string, string[]> {
  const result: Record<string, string[]> = {}
  for (const question of questionSet.questions) {
    const recommended = question.options.filter((option) => option.recommended).map((option) => option.id)
    result[question.id] = recommended.length > 0 ? recommended : question.options.slice(0, 1).map((option) => option.id)
  }
  return result
}

export function formatPlanQuestionAnswerMessage(answers: PlanQuestionAnswer[]) {
  const payload = {
    answers: answers.map((answer) => ({
      question_id: answer.questionId,
      question: answer.question,
      selected_options: answer.selectedOptions.map((option) => ({
        id: option.id,
        label: option.label,
        description: option.description || '',
      })),
      custom_answer: answer.customAnswer || '',
    })),
  }
  return `[Plan Mode question answers]\n<plan_question_answers>\n${JSON.stringify(payload, null, 2)}\n</plan_question_answers>\n\n请基于以上回答继续完善计划；如果仍有关键不确定性，请继续提问，否则输出最终 <proposed_plan>。`
}

export function formatPlanQuestionAnswerPreview(answers: PlanQuestionAnswer[]) {
  return answers.map((answer) => {
    const selected = answer.selectedOptions.map((option) => option.label).join(', ') || '未选择 / No option'
    const custom = answer.customAnswer?.trim()
    return custom ? `${answer.question}\n${selected}\n${custom}` : `${answer.question}\n${selected}`
  }).join('\n\n')
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

export function boundedPlanDisplay(content: string, maxChars = PLAN_CARD_DISPLAY_CHARS) {
  const chars = Array.from(content.trim())
  if (chars.length <= maxChars) {
    return { content: content.trim(), truncated: false }
  }
  return {
    content: `${chars.slice(0, maxChars).join('').trimEnd()}\n\n...\n[Plan 展示已截断，仅显示前 ${maxChars} 字符]`,
    truncated: true,
  }
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
