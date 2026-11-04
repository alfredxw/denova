import { installDesktopFetch } from './lib/desktop-fetch'
import { StrictMode, Suspense, useEffect } from 'react'
import loadable from '@loadable/component'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { ThemeProvider, useTheme } from 'next-themes'
import { setConfiguredLocale } from '@/i18n'
import { useTranslation } from 'react-i18next'
import './index.css'
import './mobile.css'
import './desktop.css'
import { RemoteAccessGate } from '@/features/remote-access/RemoteAccessGate'
import { LoadingState } from '@/components/common/LoadingState'

import { RuntimeErrorBoundary } from '@/components/RuntimeErrorBoundary'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { queryClient } from '@/lib/query-client'
import { installGlobalRuntimeLoggers, recordRuntimeLog, scheduleWhiteScreenCheck } from '@/lib/runtimeLog'
import { fetchSettings } from '@/features/settings/api'
import { applyFontSettings, fontSettingsFromEffective } from '@/features/settings/font-variables'
import { AgentApprovalProvider } from '@/features/agent-approval/AgentApprovalProvider'

// Fetch public application code while authentication is checked. Protected
// queries still start only after the access gate has authenticated the browser.
const App = loadable(() => import('./App'), { fallback: <AppLoadingState /> })
App.preload()

installGlobalRuntimeLoggers()
installDesktopFetch()

if (window.denovaDesktop) document.documentElement.dataset.desktopPlatform = window.denovaDesktop.platform

const root = document.getElementById('root')
if (!root) {
  recordRuntimeLog({
    type: 'startup',
    message: '前端启动失败',
    reason: 'root 节点不存在',
  })
  throw new Error('Root element not found')
}

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider attribute="data-theme" defaultTheme="dark" enableSystem themes={['light', 'dark']}>
        <TooltipProvider>
          <RuntimeErrorBoundary>
            <RemoteAccessGate>
              <AuthenticatedApp />
            </RemoteAccessGate>
            <Toaster richColors closeButton />
          </RuntimeErrorBoundary>
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
)

scheduleWhiteScreenCheck(root)

function AuthenticatedApp() {
  const { i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  useEffect(() => { void bootstrapSettings() }, [])
  useEffect(() => {
    if (!window.denovaDesktop) return
    void window.denovaDesktop.setAppearance({ locale: i18n.language === 'zh-CN' ? 'zh-CN' : 'en-US', theme: resolvedTheme === 'light' ? 'light' : 'dark' })
      .catch(error => console.warn('[desktop] appearance sync failed', error))
  }, [i18n.language, resolvedTheme])
  return (
    <Suspense fallback={<AppLoadingState />}>
      <AgentApprovalProvider><App /></AgentApprovalProvider>
    </Suspense>
  )
}

function AppLoadingState() {
  const { t } = useTranslation()
  return <LoadingState data-nova-app-shell="true" label={t('remoteAccess.connecting')} />
}

async function bootstrapSettings() {
  try {
    const settings = await fetchSettings()
    setConfiguredLocale(settings?.effective?.language)
    applyFontSettings(fontSettingsFromEffective(settings?.effective))
  } catch (error) {
    console.warn('[startup] Failed to preload UI settings; using local cache or browser defaults', error)
  }
}
