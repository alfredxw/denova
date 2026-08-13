import type { TFunction } from 'i18next'
import { APIError, withErrorLogID } from '@/lib/api-client/client'

type WorkspaceChangeFallbackKey = 'changes.operationFailed' | 'changes.loadFailed'

/** Keeps server diagnostics in logs while presenting localized, stable copy. */
export function workspaceChangeErrorMessage(t: TFunction, reason: unknown, fallbackKey: WorkspaceChangeFallbackKey = 'changes.operationFailed'): string {
  if (reason instanceof APIError && reason.code) {
    return withErrorLogID(t(`changes.error.${reason.code}`, { defaultValue: t(fallbackKey) }), reason)
  }
  return withErrorLogID(t(fallbackKey), reason)
}

export function logWorkspaceChangeError(scope: string, reason: unknown): void {
  if (reason instanceof APIError) {
    console.warn(scope, { status: reason.status, code: reason.code, details: reason.details, error: reason })
    return
  }
  console.warn(scope, { error: reason })
}
