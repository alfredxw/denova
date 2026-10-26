import type { UpdateNotesRange } from './types'

export const UPDATE_CHECK_RESULT_EVENT = 'nova:update-check-result'
export const UPDATE_NOTES_EVENT = 'nova:update-notes'

export function requestUpdateNotes(range: UpdateNotesRange) {
  window.dispatchEvent(new CustomEvent<UpdateNotesRange>(UPDATE_NOTES_EVENT, { detail: range }))
}

export function notifyUpdateCheckResult<T>(result: T) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent<T>(UPDATE_CHECK_RESULT_EVENT, { detail: result }))
}
