import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import '@/i18n'
import './index.css'
import App from './App'
import { RuntimeErrorBoundary } from '@/components/RuntimeErrorBoundary'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { queryClient } from '@/lib/query-client'
import { installGlobalRuntimeLoggers, recordRuntimeLog, scheduleWhiteScreenCheck } from '@/lib/runtimeLog'

installGlobalRuntimeLoggers()

const root = document.getElementById('root')
if (!root) {
  recordRuntimeLog({
    type: 'startup',
    message: '前端启动失败',
    reason: 'root 节点不存在',
  })
  throw new Error('root 节点不存在')
}

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <RuntimeErrorBoundary>
          <App />
          <Toaster richColors closeButton />
        </RuntimeErrorBoundary>
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
)

scheduleWhiteScreenCheck(root)
