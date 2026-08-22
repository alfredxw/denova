import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { ThemeProvider } from 'next-themes'
import { setConfiguredLocale } from '@/i18n'
import './index.css'
import App from './App'
import { RuntimeErrorBoundary } from '@/components/RuntimeErrorBoundary'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { queryClient } from '@/lib/query-client'
import { installGlobalRuntimeLoggers, recordRuntimeLog, scheduleWhiteScreenCheck } from '@/lib/runtimeLog'
import { fetchSettings } from '@/features/settings/api'
import { applyFontSettings, fontSettingsFromEffective } from '@/features/settings/font-variables'
import { AgentApprovalProvider } from '@/features/agent-approval/AgentApprovalProvider'

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
            <AgentApprovalProvider>
              <App />
            </AgentApprovalProvider>
            <Toaster richColors closeButton />
          </RuntimeErrorBoundary>
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
)

scheduleWhiteScreenCheck(root)
void bootstrapSettings()

async function bootstrapSettings() {
  try {
    const settings = await fetchSettings()
    setConfiguredLocale(settings?.effective?.language)
    applyFontSettings(fontSettingsFromEffective(settings?.effective))
  } catch (error) {
    console.warn('[startup] Failed to preload UI settings; using local cache or browser defaults', error)
  }
}
