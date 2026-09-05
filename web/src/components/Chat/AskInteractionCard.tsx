import { useEffect, useState } from 'react'
import { ChevronLeft, ChevronRight, Loader2, MessageCircleQuestion } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AgentAskAnswer, AgentAskInteraction, AgentAskQuestion, AgentAskResolution, AskChatMessage, ToolCallChatMessage } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Textarea } from '@/components/ui/textarea'
import type { AgentAskResolveAction } from '@/lib/agent-ask'
import { toolPresentationKind } from '@/lib/tool-presentation'
import { ToolInspectorButton } from './ToolInspector'

export type AskResolveAction = AgentAskResolveAction

export type AskInteractionMessage = AskChatMessage | ToolCallChatMessage
export type AskInteractionResolver = (message: AskInteractionMessage, action: AskResolveAction) => Promise<AgentAskResolution>

interface AskInteractionCardProps {
  message: AskInteractionMessage
  onResolve?: AskInteractionResolver
}

interface AskDraft {
  selectedOptionIDs: string[]
  customInput: string
}

/** Renders the durable Session interaction rather than the speculative model
 * tool frame, so remounting can answer the same call ID. */
export function AskInteractionCard({ message, onResolve }: AskInteractionCardProps) {
  const { t } = useTranslation()
  const interaction = message.ask || parseAskToolInput(message)
  const [questionIndex, setQuestionIndex] = useState(0)
  const [drafts, setDrafts] = useState<Record<string, AskDraft>>(() => askDrafts(interaction?.questions || []))
  const [localResolution, setLocalResolution] = useState<AgentAskResolution | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [expanded, setExpanded] = useState(interaction?.status === 'pending')

  useEffect(() => {
    setQuestionIndex(0)
    setDrafts(askDrafts(interaction?.questions || []))
    setLocalResolution(null)
    setSubmitting(false)
    setError('')
    setExpanded(interaction?.status === 'pending')
  }, [interaction?.id, interaction?.status])

  if (!interaction?.questions.length) return null

  const questions = interaction.questions
  const currentIndex = Math.min(questionIndex, questions.length - 1)
  const question = questions[currentIndex]
  const draft = drafts[question.id] || emptyAskDraft()
  const status = localResolution?.status || interaction.status
  const resolvedAnswers = localResolution?.answers || interaction.answers || []
  const pending = status === 'pending'

  const updateDraft = (next: AskDraft) => {
    setDrafts((current) => ({ ...current, [question.id]: next }))
    setError('')
  }

  const chooseOption = (optionID: string, checked: boolean) => {
    if (!question.multi_select) {
      updateDraft({ ...draft, selectedOptionIDs: checked ? [optionID] : [] })
      return
    }
    const selected = checked
      ? Array.from(new Set([...draft.selectedOptionIDs, optionID]))
      : draft.selectedOptionIDs.filter((id) => id !== optionID)
    updateDraft({ ...draft, selectedOptionIDs: selected })
  }

  const validateCurrent = () => {
    if (askDraftValid(question, draft)) return true
    setError(t('chat.ask.answerRequired'))
    return false
  }

  const submit = async () => {
    if (!onResolve || submitting || !validateCurrent()) return
    const invalidIndex = questions.findIndex((item) => !askDraftValid(item, drafts[item.id] || emptyAskDraft()))
    if (invalidIndex >= 0) {
      setQuestionIndex(invalidIndex)
      setError(t('chat.ask.answerRequired'))
      return
    }
    setSubmitting(true)
    setError('')
    try {
      const resolution = await onResolve(message, {
        status: 'answered',
        answers: questions.map((item) => askAnswer(item, drafts[item.id])),
      })
      setLocalResolution(resolution)
      setExpanded(false)
    } catch {
      setError(t('chat.ask.submitFailed'))
    } finally {
      setSubmitting(false)
    }
  }

  const cancel = async () => {
    if (!onResolve || submitting) return
    setSubmitting(true)
    setError('')
    try {
      const resolution = await onResolve(message, { status: 'cancelled' })
      setLocalResolution(resolution)
      setExpanded(false)
    } catch {
      setError(t('chat.ask.cancelFailed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex justify-start">
      <Collapsible className="w-full" open={pending || expanded} onOpenChange={setExpanded}>
        <section className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]" aria-label={t('chat.ask.title')}>
          <header className="flex min-w-0 items-center transition-colors hover:bg-[var(--nova-hover)]">
            <CollapsibleTrigger asChild disabled={pending}>
              <button type="button" className={`flex min-h-11 min-w-0 flex-1 items-center gap-2 px-3 py-2.5 text-left ${pending ? 'cursor-default' : 'cursor-pointer'}`}>
                <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
                  <MessageCircleQuestion className="h-4 w-4" />
                </span>
                <span className="min-w-0 flex-1 font-medium text-[var(--nova-text)]">{t('chat.ask.title')}</span>
                <span className="shrink-0 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-[11px] text-[var(--nova-text-faint)]">
                  {pending ? t('chat.ask.waiting') : status === 'answered' ? t('chat.ask.answered') : t('chat.ask.cancelled')}
                </span>
                {!pending && <ChevronRight className={`h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform ${expanded ? 'rotate-90' : ''}`} />}
              </button>
            </CollapsibleTrigger>
            {(pending || expanded) && <ToolInspectorButton className="mr-2" />}
          </header>

          <CollapsibleContent>
            {pending ? (
              <div className="border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3">
                {questions.length > 1 && (
                  <div className="mb-2 text-[11px] text-[var(--nova-text-faint)]">
                    {t('chat.ask.progress', { current: currentIndex + 1, total: questions.length })}
                  </div>
                )}
                <fieldset disabled={submitting} className="m-0 min-w-0 border-0 p-0">
                  <legend className="mb-2 block text-sm font-medium leading-5 text-[var(--nova-text)]">{question.question}</legend>
                  {question.options?.length ? (
                    <div className="grid gap-2">
                      {[
                        ...question.options,
                        ...(interaction.allow_other === false ? [] : [{ id: 'other', label: t('chat.ask.other') }]),
                      ].map((option) => {
                        const checked = draft.selectedOptionIDs.includes(option.id)
                        const recommended = option.id === question.recommended_option_id
                        return (
                          <label key={option.id} className={`flex cursor-pointer items-start gap-2 rounded-md border px-2.5 py-2 transition-colors ${checked ? 'border-[var(--nova-text-muted)] bg-[var(--nova-hover)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface)] hover:bg-[var(--nova-hover)]'}`}>
                            <input
                              type={question.multi_select ? 'checkbox' : 'radio'}
                              name={`ask-${interaction.id}-${question.id}`}
                              checked={checked}
                              onChange={(event) => chooseOption(option.id, event.target.checked)}
                              className="mt-0.5 accent-[var(--nova-accent)]"
                            />
                            <span className="min-w-0 flex-1">
                              <span className="flex flex-wrap items-center gap-1.5 text-[var(--nova-text)]">
                                {option.label}
                                {recommended && <span className="rounded-full bg-[var(--nova-hover)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('chat.ask.recommended')}</span>}
                              </span>
                              {'description' in option && option.description && <span className="mt-0.5 block leading-4 text-[var(--nova-text-faint)]">{option.description}</span>}
                            </span>
                          </label>
                        )
                      })}
                    </div>
                  ) : null}
                  {(!question.options?.length || draft.selectedOptionIDs.includes('other')) && (
                    <Textarea
                      value={draft.customInput}
                      onChange={(event) => updateDraft({ ...draft, customInput: event.target.value })}
                      placeholder={question.options?.length ? t('chat.ask.otherPlaceholder') : t('chat.ask.freePlaceholder')}
                      aria-label={question.options?.length ? t('chat.ask.other') : question.question}
                      className="mt-2 min-h-20 resize-y bg-[var(--nova-surface)] text-sm"
                    />
                  )}
                </fieldset>
                {error && <p role="alert" className="m-0 mt-2 text-[11px] text-red-400">{error}</p>}
                <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                  <Button type="button" size="sm" variant="ghost" disabled={!onResolve || submitting} onClick={() => void cancel()}>
                    {t('chat.ask.cancel')}
                  </Button>
                  <div className="flex items-center gap-2">
                    {currentIndex > 0 && (
                      <Button type="button" size="sm" variant="outline" disabled={submitting} onClick={() => { setQuestionIndex(currentIndex - 1); setError('') }}>
                        <ChevronLeft className="h-3.5 w-3.5" />
                        {t('chat.ask.back')}
                      </Button>
                    )}
                    {currentIndex < questions.length - 1 ? (
                      <Button type="button" size="sm" disabled={submitting} onClick={() => { if (validateCurrent()) setQuestionIndex(currentIndex + 1) }}>
                        {t('chat.ask.next')}
                        <ChevronRight className="h-3.5 w-3.5" />
                      </Button>
                    ) : (
                      <Button type="button" size="sm" disabled={!onResolve || submitting} onClick={() => void submit()}>
                        {submitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                        {submitting ? t('chat.ask.submitting') : t('chat.ask.submit')}
                      </Button>
                    )}
                  </div>
                </div>
              </div>
            ) : (
              <div className="grid gap-2 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3">
                {status === 'answered' ? resolvedAnswers.map((answer) => (
                  <div key={answer.question_id} className="rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2">
                    <div className="text-[var(--nova-text-faint)]">{answer.question}</div>
                    <div className="mt-1 text-[var(--nova-text)]">{askAnswerSummary(answer, t('chat.ask.other'))}</div>
                  </div>
                )) : (
                  <div className="text-[var(--nova-text-muted)]">{t('chat.ask.cancelledDescription')}</div>
                )}
              </div>
            )}
          </CollapsibleContent>
        </section>
      </Collapsible>
    </div>
  )
}

function emptyAskDraft(): AskDraft {
  return { selectedOptionIDs: [], customInput: '' }
}

function askDrafts(questions: AgentAskQuestion[]): Record<string, AskDraft> {
  return Object.fromEntries(questions.map((question) => [question.id, emptyAskDraft()]))
}

function askDraftValid(question: AgentAskQuestion, draft: AskDraft) {
  if (!question.options?.length) return Boolean(draft.customInput.trim())
  if (draft.selectedOptionIDs.length === 0) return false
  if (!question.multi_select && draft.selectedOptionIDs.length !== 1) return false
  return !draft.selectedOptionIDs.includes('other') || Boolean(draft.customInput.trim())
}

function askAnswer(question: AgentAskQuestion, draft = emptyAskDraft()): AgentAskAnswer {
  return {
    question_id: question.id,
    ...(question.options?.length ? { selected_option_ids: draft.selectedOptionIDs } : {}),
    ...((!question.options?.length || draft.selectedOptionIDs.includes('other')) ? { custom_input: draft.customInput.trim() } : {}),
  }
}

function askAnswerSummary(answer: NonNullable<AgentAskInteraction['answers']>[number], otherLabel: string) {
  const selections = (answer.selected_options || []).map((option) => option.id === 'other' ? otherLabel : option.label)
  if (answer.custom_input) selections.push(answer.custom_input)
  return selections.join(' · ')
}

function parseAskToolInput(message: AskInteractionMessage): AgentAskInteraction | undefined {
  if (message.role !== 'tool_call' || toolPresentationKind(message, 'call') !== 'interaction' || !message.id) return undefined
  try {
    const input = JSON.parse(message.args || '') as { questions?: unknown }
    if (!Array.isArray(input.questions)) return undefined
    const questions = input.questions
      .map((value): AgentAskQuestion | undefined => {
        if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
        const raw = value as Record<string, unknown>
        const id = typeof raw.id === 'string' ? raw.id.trim() : ''
        const question = typeof raw.prompt === 'string' ? raw.prompt.trim() : ''
        if (!id || !question) return undefined
        const options = Array.isArray(raw.options)
          ? raw.options.flatMap((option) => {
            if (!option || typeof option !== 'object' || Array.isArray(option)) return []
            const item = option as Record<string, unknown>
            const optionID = typeof item.value === 'string' ? item.value.trim() : ''
            const label = typeof item.label === 'string' ? item.label.trim() : ''
            if (!optionID || !label) return []
            return [{
              id: optionID,
              label,
              ...(typeof item.description === 'string' && item.description.trim() ? { description: item.description.trim() } : {}),
              recommended: item.recommended === true,
            }]
          })
          : []
        const recommended = options.find((option) => option.recommended)?.id
        return {
          id,
          question,
          ...(options.length ? { options: options.map(({ recommended: _, ...option }) => option) } : {}),
          ...(raw.multiple === true ? { multi_select: true } : {}),
          ...(recommended ? { recommended_option_id: recommended } : {}),
        }
      })
      .filter((question): question is AgentAskQuestion => Boolean(question))
    if (questions.length === 0) return undefined
    return {
      schema: 'ask.pending.v1', id: message.id, tool_call_id: message.id,
      agent_kind: message.agent_kind || 'ide', status: 'pending', questions,
    }
  } catch {
    return undefined
  }
}
