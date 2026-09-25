import { errorMessage } from '@/lib/error-diagnostics'

export const MODEL_IMAGE_INPUT_REJECTED_CODE = 'agent_runtime.model_image_input_rejected'
export const MODEL_REQUEST_TOO_LARGE_CODE = 'agent_runtime.model_request_too_large'
export const MODEL_OUTPUT_TRUNCATED_CODE = 'agent_runtime.model_output_truncated'
export const MODEL_CONTEXT_WINDOW_EXCEEDED_CODE = 'agent_runtime.model_context_window_exceeded'
export const MODEL_OUTPUT_FILTERED_CODE = 'agent_runtime.model_output_filtered'
export const MODEL_OUTPUT_INCOMPLETE_CODE = 'agent_runtime.model_output_incomplete'

type Translate = (key: string) => string

const modelErrorTranslationKeys: Record<string, string> = {
  [MODEL_IMAGE_INPUT_REJECTED_CODE]: 'common.modelImageInputRejected',
  [MODEL_REQUEST_TOO_LARGE_CODE]: 'common.modelRequestTooLarge',
  [MODEL_OUTPUT_TRUNCATED_CODE]: 'common.modelOutputTruncated',
  [MODEL_CONTEXT_WINDOW_EXCEEDED_CODE]: 'common.modelContextWindowExceeded',
  [MODEL_OUTPUT_FILTERED_CODE]: 'common.modelOutputFiltered',
  [MODEL_OUTPUT_INCOMPLETE_CODE]: 'common.modelOutputIncomplete',
}

export function localizeAgentRuntimeReason(reason: unknown, fallback: string, t: Translate) {
  const value = typeof reason === 'string' ? reason.trim() : ''
  const translationKey = modelErrorTranslationKeys[value]
  if (translationKey) return t(translationKey)
  return value || fallback
}

export function localizeAgentRuntimeError(data: Record<string, unknown>, fallback: string, t: Translate) {
  const translationKey = typeof data.error_key === 'string' ? data.error_key : typeof data.code === 'string' ? modelErrorTranslationKeys[data.code] : undefined
  const reason = [data.content, data.message, data.error]
    .find((value): value is string => typeof value === 'string' && Boolean(value.trim()))
  const summary = translationKey ? t(translationKey) : localizeAgentRuntimeReason(reason, fallback, t)
  return errorMessage({ ...data, summary })
}
