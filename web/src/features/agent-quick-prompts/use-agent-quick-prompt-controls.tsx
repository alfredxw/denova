import { useMemo, useState } from 'react'
import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ComposerMenuItem } from '@/components/Chat/ComposerMenuRow'
import type { AgentQuickPromptSettings } from '@/features/settings/types'
import { AgentQuickPrompts } from './AgentQuickPrompts'
import { agentQuickPromptDefaults, type AgentQuickPromptScope } from './defaults'
import { QuickPromptSettingsDialog } from './QuickPromptSettingsDialog'
import { useAgentQuickPrompts } from './use-agent-quick-prompts'

const NO_PROMPTS: AgentQuickPromptSettings[] = []

/** One page-scoped list owns the starter cards, persistent settings entry, and slash suggestions. */
export function useAgentQuickPromptControls({ scope, disabled, onFill, onSend }: {
  scope: AgentQuickPromptScope | undefined
  disabled: boolean
  onFill: (prompt: string) => void
  onSend: (prompt: string) => void | Promise<unknown>
}) {
  const { t } = useTranslation()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const defaults = useMemo(() => scope ? agentQuickPromptDefaults(t, scope) : null, [scope, t])
  const settings = useAgentQuickPrompts(scope, defaults?.prompts ?? NO_PROMPTS)
  const loading = disabled || settings.loading

  return {
    commands: scope && settings.showInCommands ? settings.prompts : NO_PROMPTS,
    cards: defaults ? (
      <AgentQuickPrompts
        title={defaults.title}
        scopeLabel={defaults.scopeLabel}
        prompts={settings.prompts}
        disabled={loading}
        onOpenSettings={() => setSettingsOpen(true)}
        onFill={onFill}
        onSend={onSend}
      />
    ) : null,
    menuItem: defaults ? (
      <ComposerMenuItem
        icon={Sparkles}
        label={t('chat.quick.settings.open', { scope: defaults.scopeLabel })}
        disabled={loading}
        onSelect={() => setSettingsOpen(true)}
      />
    ) : null,
    dialog: defaults ? (
      <QuickPromptSettingsDialog
        key={scope}
        open={settingsOpen}
        scopeLabel={defaults.scopeLabel}
        prompts={settings.prompts}
        defaults={defaults.prompts}
        customized={settings.customized}
        showInCommands={settings.showInCommands}
        onOpenChange={setSettingsOpen}
        onSave={async (changes) => { await settings.save(changes) }}
      />
    ) : null,
  }
}
