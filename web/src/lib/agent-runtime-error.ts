export const MODEL_OUTPUT_TRUNCATED_CODE = 'agent_runtime.model_output_truncated'

type Translate = (key: string) => string

export function localizeAgentRuntimeReason(reason: unknown, fallback: string, t: Translate) {
  const value = typeof reason === 'string' ? reason.trim() : ''
  if (value === MODEL_OUTPUT_TRUNCATED_CODE) return t('common.modelOutputTruncated')
  return value || fallback
}

export function localizeAgentRuntimeError(data: Record<string, unknown>, fallback: string, t: Translate) {
  if (data.code === MODEL_OUTPUT_TRUNCATED_CODE) return t('common.modelOutputTruncated')
  const reason = [data.content, data.message, data.error]
    .find((value): value is string => typeof value === 'string' && Boolean(value.trim()))
  return localizeAgentRuntimeReason(reason, fallback, t)
}
