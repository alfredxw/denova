import { useEffect, useMemo, useRef, useState } from 'react'
import { Check, ChevronDown, Loader2, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { discoverModels } from './api'
import { MODEL_PROTOCOL_ANTHROPIC_MESSAGES, MODEL_PROTOCOL_CHAT_COMPLETIONS, MODEL_PROTOCOL_RESPONSES } from './model-profiles'
import type { ModelEndpointSettings, ModelInfo, ModelProfileSettings } from './types'

type DiscoveryState =
  | { status: 'idle'; models: ModelInfo[] }
  | { status: 'loading'; models: ModelInfo[] }
  | { status: 'success'; models: ModelInfo[] }
  | { status: 'error'; models: ModelInfo[]; message: string }

export function ModelDiscoveryInput({ endpoint, profile, defaultProtocol, value, placeholder, onChange }: {
  endpoint: ModelEndpointSettings
  profile: ModelProfileSettings
  defaultProtocol?: string
  value: string
  placeholder: string
  onChange: (model: string) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [state, setState] = useState<DiscoveryState>({ status: 'idle', models: [] })
  const requestRef = useRef<AbortController | null>(null)
  const protocol = endpoint.protocol?.trim() || defaultProtocol?.trim() || ''
  const supported = protocol === MODEL_PROTOCOL_CHAT_COMPLETIONS
    || protocol === MODEL_PROTOCOL_RESPONSES
    || protocol === MODEL_PROTOCOL_ANTHROPIC_MESSAGES
  const routeFingerprint = useMemo(() => JSON.stringify({
    id: endpoint.id,
    provider: endpoint.provider,
    protocol,
    api_key: endpoint.api_key,
    base_url: endpoint.base_url,
    headers: endpoint.headers,
    protocol_options: endpoint.protocol_options,
  }), [endpoint.api_key, endpoint.base_url, endpoint.headers, endpoint.id, endpoint.protocol_options, endpoint.provider, protocol])

  useEffect(() => {
    requestRef.current?.abort()
    requestRef.current = null
    setState({ status: 'idle', models: [] })
    setOpen(false)
  }, [routeFingerprint])

  useEffect(() => () => requestRef.current?.abort(), [])

  const load = async () => {
    requestRef.current?.abort()
    const request = new AbortController()
    requestRef.current = request
    setState((current) => ({ status: 'loading', models: current.models }))
    try {
      const result = await discoverModels(endpoint, profile, request.signal)
      if (requestRef.current === request) {
        requestRef.current = null
        setState({ status: 'success', models: result.models ?? [] })
      }
    } catch (error) {
      if (request.signal.aborted) return
      requestRef.current = null
      setState((current) => ({
        status: 'error',
        models: current.models,
        message: error instanceof Error ? error.message : String(error),
      }))
    }
  }

  const available = supported && Boolean(endpoint.provider?.trim())
  const actionLabel = available
    ? t('settings.model.discoveryAction')
    : t('settings.model.discoveryUnsupported')

  return (
    <div className="flex min-w-0 gap-1">
      <Input
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
        className="min-w-0 flex-1"
      />
      <Popover
        open={open}
        onOpenChange={(nextOpen) => {
          setOpen(nextOpen)
          if (nextOpen && state.status === 'idle') void load()
        }}
      >
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            disabled={!available}
            aria-label={actionLabel}
            title={actionLabel}
            className="shrink-0"
          >
            {state.status === 'loading' ? <Loader2 className="animate-spin" /> : <ChevronDown />}
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="end"
          sideOffset={4}
          collisionPadding={8}
          className="nova-panel w-[min(24rem,calc(100vw-1rem))] gap-1 border p-1 text-[var(--nova-text)]"
        >
          <div className="flex items-center justify-between gap-2 px-2 py-1 text-[11px] text-[var(--nova-text-faint)]">
            <span>{t('settings.model.discoveryHint')}</span>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              disabled={state.status === 'loading'}
              aria-label={t('settings.model.discoveryRefresh')}
              onClick={() => void load()}
            >
              <RefreshCw className={cn(state.status === 'loading' && 'animate-spin')} />
            </Button>
          </div>
          {state.status === 'loading' && state.models.length === 0 && (
            <div className="px-2 py-3 text-xs text-[var(--nova-text-faint)]">{t('settings.model.discoveryLoading')}</div>
          )}
          {state.status === 'error' && state.models.length === 0 && (
            <div className="px-2 py-2 text-xs leading-5 text-red-600 dark:text-red-400" role="alert">
              {t('settings.model.discoveryFailure', { error: state.message })}
            </div>
          )}
          {state.status === 'success' && state.models.length === 0 && (
            <div className="px-2 py-3 text-xs text-[var(--nova-text-faint)]">{t('settings.model.discoveryEmpty')}</div>
          )}
          {state.models.length > 0 && (
            <div role="listbox" aria-label={t('settings.model.discoveryList')} className="max-h-64 overflow-y-auto overscroll-contain [scrollbar-gutter:stable]">
              {state.models.map((model) => (
                <button
                  key={model.id}
                  type="button"
                  role="option"
                  aria-selected={model.id === value.trim()}
                  className={cn(
                    'flex h-8 w-full min-w-0 items-center gap-2 rounded-md px-2 text-left text-xs',
                    model.id === value.trim()
                      ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
                      : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
                  )}
                  onClick={() => {
                    onChange(model.id)
                    setOpen(false)
                  }}
                >
                  <span className="min-w-0 flex-1 truncate" title={model.display_name || model.id}>
                    {model.display_name || model.id}
                  </span>
                  {model.display_name && (
                    <span className="max-w-[55%] shrink truncate font-mono text-[11px] text-[var(--nova-text-faint)]" title={model.id}>
                      {model.id}
                    </span>
                  )}
                  {!model.display_name && model.owned_by && (
                    <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{model.owned_by}</span>
                  )}
                  {model.id === value.trim() && <Check className="size-3.5 shrink-0" />}
                </button>
              ))}
            </div>
          )}
        </PopoverContent>
      </Popover>
    </div>
  )
}
