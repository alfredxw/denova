import { lazy, StrictMode, Suspense, useEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { ThemeProvider } from 'next-themes'
import { setConfiguredLocale } from '@/i18n'
import './index.css'
import './mobile.css'
import { RemoteAccessGate } from '@/features/remote-access/RemoteAccessGate'
import { LoadingState } from '@/components/common/LoadingState'
import i18next from '@/i18n'

import { RuntimeErrorBoundary } from '@/components/RuntimeErrorBoundary'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { queryClient } from '@/lib/query-client'
import { installGlobalRuntimeLoggers, recordRuntimeLog, scheduleWhiteScreenCheck } from '@/lib/runtimeLog'
import { fetchSettings } from '@/features/settings/api'
import { applyFontSettings, fontSettingsFromEffective } from '@/features/settings/font-variables'
import { AgentApprovalProvider } from '@/features/agent-approval/AgentApprovalProvider'

const App = lazy(() => import('./App'))

installGlobalRuntimeLoggers()

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
  useEffect(() => { void bootstrapSettings() }, [])
  return (
    <Suspense fallback={<LoadingState data-nova-app-shell="true" label={i18next.t('remoteAccess.connecting')} />}>
      <AgentApprovalProvider><App /></AgentApprovalProvider>
    </Suspense>
  )
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
