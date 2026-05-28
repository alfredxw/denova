import { Component, type ErrorInfo, type ReactNode } from 'react'
import { normalizeRuntimeError, recordRuntimeLog } from '@/lib/runtimeLog'
import { Button } from '@/components/ui/button'

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
        <div className="flex h-screen w-screen items-center justify-center bg-[#18191b] px-6 text-[#d7dbe2]">
          <div className="max-w-xl rounded border border-[#5c2a2a] bg-[#241f1f] p-5 shadow-2xl">
            <div className="text-base font-semibold text-[#ff8f8f]">Nova 前端渲染异常</div>
            <div className="mt-2 text-sm leading-6 text-[#c8ccd4]">
              已记录崩溃原因到浏览器控制台和 <code>localStorage.nova.runtime.logs</code>。
            </div>
            <pre className="mt-3 max-h-40 overflow-auto rounded bg-[#18191b] p-3 text-xs text-[#ffb3b3] whitespace-pre-wrap">
              {this.state.errorMessage}
            </pre>
            <Button
              type="button"
              size="sm"
              className="mt-4 bg-[#4a4d54] text-white hover:bg-[#5a5d64]"
              onClick={() => window.location.reload()}
            >
              刷新页面
            </Button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
