export const MODEL_OUTPUT_TRUNCATED_CODE = 'agent_runtime.model_output_truncated'
export const MODEL_CONTEXT_WINDOW_EXCEEDED_CODE = 'agent_runtime.model_context_window_exceeded'
export const MODEL_OUTPUT_FILTERED_CODE = 'agent_runtime.model_output_filtered'
export const MODEL_OUTPUT_INCOMPLETE_CODE = 'agent_runtime.model_output_incomplete'

type Translate = (key: string) => string

const modelIncompleteTranslationKeys: Record<string, string> = {
  [MODEL_OUTPUT_TRUNCATED_CODE]: 'common.modelOutputTruncated',
  [MODEL_CONTEXT_WINDOW_EXCEEDED_CODE]: 'common.modelContextWindowExceeded',
  [MODEL_OUTPUT_FILTERED_CODE]: 'common.modelOutputFiltered',
  [MODEL_OUTPUT_INCOMPLETE_CODE]: 'common.modelOutputIncomplete',
}

export function localizeAgentRuntimeReason(reason: unknown, fallback: string, t: Translate) {
  const value = typeof reason === 'string' ? reason.trim() : ''
  const translationKey = modelIncompleteTranslationKeys[value]
  if (translationKey) return t(translationKey)
  return value || fallback
}

export function localizeAgentRuntimeError(data: Record<string, unknown>, fallback: string, t: Translate) {
  if (typeof data.code === 'string') {
    const translationKey = modelIncompleteTranslationKeys[data.code]
    if (translationKey) return t(translationKey)
  }
  const reason = [data.content, data.message, data.error]
    .find((value): value is string => typeof value === 'string' && Boolean(value.trim()))
  return localizeAgentRuntimeReason(reason, fallback, t)
}
