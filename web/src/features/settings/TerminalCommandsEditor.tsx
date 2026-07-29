import { ChevronDown, ChevronUp, Plus, TerminalSquare, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import type { Settings, TerminalCommandSettings } from './types'

const TERMINAL_COMMAND_PRESET_IDS = new Set(['codex', 'claude'])

/** Resolves the atomic user-owned registry shown by the editor. */
export function terminalCommandsForEditor(draft: Settings, effective: Settings): TerminalCommandSettings[] {
  const commands = draft.terminal_commands ?? effective.terminal_commands ?? []
  return commands.map((command) => ({ ...command }))
}

/** Edits every terminal shortcut through the same ordered registry model. */
export function TerminalCommandsEditor({ commands, onChange }: {
  commands: TerminalCommandSettings[]
  onChange: (commands: TerminalCommandSettings[]) => void
}) {
  const { t } = useTranslation()
  const updateCommand = (index: number, patch: Partial<TerminalCommandSettings>) => {
    onChange(commands.map((command, commandIndex) => (
      commandIndex === index ? { ...command, ...patch } : command
    )))
  }
  const moveCommand = (index: number, offset: -1 | 1) => {
    const target = index + offset
    if (target < 0 || target >= commands.length) return
    const reordered = [...commands]
    const moving = reordered[index]
    reordered[index] = reordered[target]
    reordered[target] = moving
    onChange(reordered)
  }
  const removeCommand = (index: number) => {
    if (TERMINAL_COMMAND_PRESET_IDS.has(commands[index]?.id)) return
    onChange(commands.filter((_, commandIndex) => commandIndex !== index))
  }
  const addCommand = () => {
    onChange([...commands, {
      id: createTerminalCommandID(),
      name: t('settings.terminal.newCommandName'),
      command: 'my-cli',
      enabled: false,
    }])
  }

  return (
    <div className="nova-settings-row rounded-md px-2 py-1.5">
      <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('settings.terminal.commands')}</div>
      <div className="flex flex-col gap-2">
        {commands.length === 0 && (
          <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2 text-[var(--nova-text-faint)]">
            {t('settings.terminal.commandsEmpty')}
          </div>
        )}
        {commands.map((command, index) => (
          <TerminalCommandCard
            key={command.id}
            command={command}
            index={index}
            count={commands.length}
            preset={TERMINAL_COMMAND_PRESET_IDS.has(command.id)}
            onUpdate={(patch) => updateCommand(index, patch)}
            onMove={(offset) => moveCommand(index, offset)}
            onRemove={() => removeCommand(index)}
          />
        ))}
        <Button type="button" onClick={addCommand} variant="outline" size="sm">
          <Plus data-icon="inline-start" />
          {t('settings.terminal.addCommand')}
        </Button>
      </div>
    </div>
  )
}

function TerminalCommandCard({ command, index, count, preset, onUpdate, onMove, onRemove }: {
  command: TerminalCommandSettings
  index: number
  count: number
  preset: boolean
  onUpdate: (patch: Partial<TerminalCommandSettings>) => void
  onMove: (offset: -1 | 1) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const hasRequiredFields = Boolean(command.name.trim() && command.command.trim())
  const displayName = command.name.trim() || t('settings.terminal.unnamedCommand')
  return (
    <div data-testid="terminal-command-card" className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      <div className="flex items-center gap-2 px-2.5 py-2">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)]">
          <TerminalSquare className="size-3.5" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-1.5">
            <span className="truncate text-xs font-medium text-[var(--nova-text)]">{displayName}</span>
            <Badge variant="outline" className="shrink-0 px-1.5 py-0 text-[10px]">
              {t(preset ? 'settings.terminal.preset' : 'settings.terminal.custom')}
            </Badge>
          </div>
          <code className="block truncate text-[11px] text-[var(--nova-text-faint)]">
            $ {command.command.trim() || t('settings.terminal.commandMissing')}
          </code>
        </div>
        <Switch
          size="sm"
          checked={command.enabled}
          onCheckedChange={(enabled) => onUpdate({ enabled })}
          aria-label={t('settings.terminal.toggleCommand', { name: displayName })}
          title={t('settings.terminal.toggleCommand', { name: displayName })}
        />
        <div className="flex shrink-0 items-center gap-0.5">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={index === 0}
            onClick={() => onMove(-1)}
            aria-label={t('settings.terminal.moveCommandUp', { name: displayName })}
            title={t('settings.terminal.moveCommandUp', { name: displayName })}
          >
            <ChevronUp />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={index === count - 1}
            onClick={() => onMove(1)}
            aria-label={t('settings.terminal.moveCommandDown', { name: displayName })}
            title={t('settings.terminal.moveCommandDown', { name: displayName })}
          >
            <ChevronDown />
          </Button>
          {!preset && (
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={onRemove}
              aria-label={t('settings.terminal.deleteCommand', { name: displayName })}
              title={t('settings.terminal.deleteCommand', { name: displayName })}
            >
              <Trash2 />
            </Button>
          )}
        </div>
      </div>
      <Separator />
      <div className="grid gap-2 p-2.5 sm:grid-cols-[minmax(10rem,0.8fr)_minmax(14rem,1.6fr)]">
        <TerminalCommandInput label={t('settings.terminal.commandName')}>
          <Input
            value={command.name}
            maxLength={128}
            aria-invalid={!command.name.trim()}
            placeholder={t('settings.terminal.commandNamePlaceholder')}
            onChange={(event) => onUpdate({ name: event.target.value })}
          />
        </TerminalCommandInput>
        <TerminalCommandInput label={t('settings.terminal.launchCommand')}>
          <Input
            value={command.command}
            aria-invalid={!command.command.trim()}
            className="font-mono"
            placeholder={t('settings.terminal.launchCommandPlaceholder')}
            onChange={(event) => onUpdate({ command: event.target.value })}
          />
        </TerminalCommandInput>
      </div>
      {!hasRequiredFields && (
        <div className="border-t border-[var(--nova-border)] px-2.5 py-1.5 text-[11px] text-destructive">
          {t('settings.terminal.commandRequired')}
        </div>
      )}
    </div>
  )
}

function TerminalCommandInput({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex min-w-0 flex-col gap-1">
      <span className="text-[11px] leading-none text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

function createTerminalCommandID() {
  const suffix = globalThis.crypto?.randomUUID?.().replaceAll('-', '')
    ?? `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`
  return `cli-${suffix.toLowerCase()}`
}
