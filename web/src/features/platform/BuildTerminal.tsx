import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { TerminalTabView } from '@/features/agent-chat/terminal/TerminalTabView'
import {
  closeTerminalSession,
  terminalAttachURL,
  type TerminalSessionInfo,
} from '@/features/agent-chat/terminal/api'
import { TerminalConnection } from '@/features/agent-chat/terminal/connection'
import type { AgentChatTerminalTab } from '@/features/agent-chat/types'

/** Build recipes run visibly in the existing PTY, with shell-specific quoting. */
export function BuildTerminal({
  projectId,
  directory,
  command,
  onClose,
  onOutput,
}: {
  projectId: string
  directory: string
  command: { command: string; args: string[] }
  onClose: () => void
  onOutput?: (output: string) => void
}) {
  const { t } = useTranslation()
  const [tab] = useState<AgentChatTerminalTab>({
    kind: 'terminal',
    id: crypto.randomUUID(),
    projectId,
    workspace: '',
    group: 'primary',
    profileId: 'shell',
    title: t('platform.build'),
  })
  const session = useRef<TerminalSessionInfo | null>(null)
  const input = useRef<TerminalConnection | null>(null)
  const decoder = useRef(new TextDecoder())
  const [ready, setReady] = useState(false)
  const [recipe, setRecipe] = useState<{
    display: string
    input: string
  } | null>(null)
  const establish = useCallback(
    (_id: string, next: TerminalSessionInfo) => {
      session.current = next
      const cmd = /(?:^|[\\/])cmd(?:\.exe)?$/i.test(next.command)
      const powershell =
        cmd || /(?:pwsh|powershell)(?:\.exe)?$/i.test(next.command)
      const legacyArguments = cmd || /powershell(?:\.exe)?$/i.test(next.command)
      const posix = /(?:bash|zsh|fish|sh)(?:\.exe)?$/i.test(next.command)
      if (!powershell && !posix) return true
      const quote = (text: string) =>
        powershell
          ? `'${text.replaceAll("'", "''")}'`
          : `'${text.replaceAll("'", "'\\''")}'`
      // Windows PowerShell rebuilds a native command line. Preserve quotes,
      // empty arguments and trailing backslashes using Windows argv quoting.
      const args = (command.args ?? []).map((text) =>
        quote(
          legacyArguments
            ? `"${text.replace(/(\\*)"/g, '$1$1\\"').replace(/(\\+)$/, '$1$1')}"`
            : text,
        ),
      )
      const display = powershell
        ? `Set-Location -LiteralPath ${quote(directory)}\n& ${quote(command.command)} ${args.join(' ')}\n`
        : `cd -- ${quote(directory)} && ${quote(command.command)} ${args.join(' ')}\n`
      let payload = display.replaceAll('\n', '\r')
      if (cmd) {
        // A cmd-hosted terminal delegates the whole recipe to PowerShell.
        // UTF-16LE encoding prevents a second shell from expanding its arguments.
        let bytes = ''
        for (let index = 0; index < display.length; index++) {
          const unit = display.charCodeAt(index)
          bytes += String.fromCharCode(unit & 255, unit >> 8)
        }
        payload = `powershell.exe -NoLogo -NoProfile -EncodedCommand ${btoa(bytes)}\r`
      }
      setRecipe({ display, input: payload })
      input.current?.close()
      input.current = new TerminalConnection(
        terminalAttachURL(next.id, next.token),
        {
          onOutput: (output) => onOutput?.(decoder.current.decode(output, { stream: true })),
          onControl: (frame) => {
            if (frame.type === 'ready') setReady(true)
          },
          onClosed: () => setReady(false),
        },
      )
      return true
    },
    [command, directory, onOutput],
  )
  useEffect(
    () => () => {
      input.current?.close()
      if (session.current)
        void closeTerminalSession(session.current.id).catch((error) =>
          console.warn('[platform] close build terminal failed', error),
        )
    },
    [],
  )
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="flex h-[85dvh] max-w-[95vw] flex-col sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle>{t('platform.build')}</DialogTitle>
          <DialogDescription>
            {t('platform.buildDescription')}
          </DialogDescription>
        </DialogHeader>
        <pre className="overflow-auto whitespace-pre-wrap break-words text-xs">
          {recipe?.display ||
            `${directory}\n${command.command} ${(command.args ?? []).join(' ')}`}
        </pre>
        <Button
          disabled={!ready || !recipe}
          onClick={() => {
            if (recipe) input.current?.send(recipe.input)
          }}
        >
          {t('platform.runBuild')}
        </Button>
        <div className="min-h-0 flex-1">
          <TerminalTabView
            tab={tab}
            active
            onSessionEstablished={establish}
            onTitleChange={() => {}}
            onStatusChange={() => {}}
          />
        </div>
      </DialogContent>
    </Dialog>
  )
}
