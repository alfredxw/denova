import type { UpdateStatus } from '@/features/settings/types'

/** Optional shell capabilities. Business data and Agent operations always use
 * the same HTTP API in both distributions. No Node or raw IPC crosses here. */
export interface DesktopBridge {
  readonly platform: string
  readonly backendOrigin: string
  chooseDirectory(initialPath?: string): Promise<{ path: string; canceled: boolean }>
  /** Owns native selection, activity checks, backend shutdown, and relaunch. */
  changeDataDirectory(): Promise<void>
  setAppearance(appearance: { locale: 'en-US' | 'zh-CN'; theme: 'dark' | 'light' | 'system' }): Promise<void>
  updates: {
    status(): Promise<UpdateStatus>
    open(): Promise<void>
    preferences(): Promise<{ automaticChecks: boolean; automaticDownloads: boolean }>
    setPreferences(value: { automaticChecks: boolean; automaticDownloads: boolean }): Promise<void>
  }
}

declare global { interface Window { denovaDesktop?: DesktopBridge } }

export function desktopBridge(): DesktopBridge | undefined {
  return window.denovaDesktop
}
