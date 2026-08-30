import { useMemo, useState } from 'react'
import { MessageCircle, SlidersHorizontal, Sparkles, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { agentQuickPromptDefaults, type AgentQuickPromptScope } from './defaults'
import { QuickPromptSettingsDialog } from './QuickPromptSettingsDialog'
import { useAgentQuickPrompts } from './use-agent-quick-prompts'

interface AgentQuickPromptsProps {
  scope: AgentQuickPromptScope
  writingTarget: string
  disabled?: boolean
  onFill: (prompt: string) => void
  onSend: (prompt: string) => void | Promise<unknown>
}

export function AgentQuickPrompts({ scope, writingTarget, disabled, onFill, onSend }: AgentQuickPromptsProps) {
  const { t } = useTranslation()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const defaults = useMemo(
    () => agentQuickPromptDefaults(t, scope, writingTarget),
    [scope, t, writingTarget],
  )
  const settings = useAgentQuickPrompts(scope, defaults.prompts)
  const visiblePrompts = settings.prompts.filter((prompt) => prompt.enabled)

  return (
    <>
      <section className="border-b bg-background p-3">
        <header className="mb-2 flex min-h-7 items-center gap-2">
          <Sparkles className="size-3.5 text-muted-foreground" aria-hidden="true" />
          <h2 className="min-w-0 flex-1 truncate text-xs font-medium text-muted-foreground">{defaults.title}</h2>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={() => setSettingsOpen(true)}
            aria-label={t('chat.quick.settings.open', { scope: defaults.scopeLabel })}
            title={t('chat.quick.settings.open', { scope: defaults.scopeLabel })}
          >
            <SlidersHorizontal />
          </Button>
        </header>
        {visiblePrompts.length > 0 ? (
          <div className="grid grid-cols-2 gap-2">
            {visiblePrompts.map((prompt) => (
              <Button
                key={prompt.id}
                type="button"
                variant="outline"
                disabled={disabled}
                className="h-auto min-w-0 justify-start px-3 py-2 text-left text-xs"
                onClick={() => {
                  if (prompt.behavior === 'send') {
                    void onSend(prompt.prompt)
                    return
                  }
                  onFill(prompt.prompt)
                }}
                aria-label={t('chat.quick.runLabel', { name: prompt.name, behavior: t(`chat.quick.behavior.${prompt.behavior}`) })}
                title={t(`chat.quick.behavior.${prompt.behavior}`)}
              >
                <MessageCircle data-icon="inline-start" />
                <span className="min-w-0 flex-1 truncate">{prompt.name}</span>
                {prompt.behavior === 'send' ? <Zap data-icon="inline-end" className="text-muted-foreground" /> : null}
              </Button>
            ))}
          </div>
        ) : (
          <button
            type="button"
            className="flex min-h-14 w-full items-center justify-center rounded-lg border border-dashed px-3 text-xs text-muted-foreground hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => setSettingsOpen(true)}
          >
            {t('chat.quick.settings.emptySection')}
          </button>
        )}
      </section>
      <QuickPromptSettingsDialog
        open={settingsOpen}
        scopeLabel={defaults.scopeLabel}
        prompts={settings.prompts}
        defaults={defaults.prompts}
        customized={settings.customized}
        onOpenChange={setSettingsOpen}
        onSave={async (prompts) => { await settings.save(prompts) }}
      />
    </>
  )
}
