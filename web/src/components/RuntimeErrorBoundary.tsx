import { Component, type ErrorInfo, type ReactNode } from 'react'
import { normalizeRuntimeError, recordRuntimeLog } from '@/lib/runtimeLog'
import { Button } from '@/components/ui/button'
import i18n from '@/i18n'

interface RuntimeErrorBoundaryProps {
  children: ReactNode
}

interface RuntimeErrorBoundaryState {
  errorMessage: string
}

/** 捕获 React 渲染崩溃，记录原因并显示兜底页面，避免用户只看到白屏。 */
export class RuntimeErrorBoundary extends Component<RuntimeErrorBoundaryProps, RuntimeErrorBoundaryState> {
  state: RuntimeErrorBoundaryState = { errorMessage: '' }

  static getDerivedStateFromError(error: unknown): RuntimeErrorBoundaryState {
    return { errorMessage: normalizeRuntimeError(error).message }
  }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    const normalized = normalizeRuntimeError(error)
    recordRuntimeLog({
      type: 'react_error',
      message: normalized.message,
      reason: 'React 渲染异常',
      stack: normalized.stack,
      componentStack: info.componentStack || undefined,
    })
  }

  render() {
    if (this.state.errorMessage) {
      return (
        <div className="flex h-dvh w-screen items-center justify-center bg-[var(--nova-bg)] px-6 text-[var(--nova-text)]">
          <div className="max-w-xl rounded border border-[var(--nova-danger-border)] bg-[var(--nova-surface)] p-5 shadow-2xl">
            <div className="text-base font-semibold text-[var(--nova-danger)]">{i18n.t('runtime.title')}</div>
            <div className="mt-2 text-sm leading-6 text-[var(--nova-text-muted)]">
              {i18n.t('runtime.description')}
            </div>
            <pre className="mt-3 max-h-40 overflow-auto rounded bg-[var(--nova-surface-2)] p-3 text-xs text-[var(--nova-danger)] whitespace-pre-wrap">
              {this.state.errorMessage}
            </pre>
            <Button
              type="button"
              size="sm"
              className="mt-4 bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]"
              onClick={() => window.location.reload()}
            >
              {i18n.t('runtime.reload')}
            </Button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
