export interface ActionableTaskNotificationOptions {
  enabled: boolean
  typeLabel: string
  projectName: string
}

/**
 * Shows a system notification for an actionable task (waiting user or failed)
 * when the user has enabled system notifications. The notification only
 * contains the task type and source project, never body, prompt, or tool
 * content. Permission is requested only after the user opted in.
 */
export async function maybeNotifyActionableTask(options: ActionableTaskNotificationOptions): Promise<boolean> {
  if (!options.enabled) return false
  if (typeof window === 'undefined' || !('Notification' in window)) return false
  let permission = window.Notification.permission
  if (permission === 'default') {
    permission = await window.Notification.requestPermission()
  }
  if (permission !== 'granted') return false
  const title = [options.typeLabel, options.projectName].filter(Boolean).join(' · ')
  if (!title) return false
  new window.Notification(title)
  return true
}
