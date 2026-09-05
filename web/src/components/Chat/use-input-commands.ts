import { useEffect, useMemo, useState } from 'react'
import { Archive, BadgeHelp, ClipboardList, Eraser, Sparkles, Target } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AgentQuickPromptSettings } from '@/features/settings/types'
import type { InputCommandOption } from './InputCommandMenu'

const COMMANDS = [
  { cmd: '/goal', descKey: 'chat.command.goal.desc', hintKey: 'chat.command.goal.hint', icon: Target },
  { cmd: '/plan', descKey: 'chat.command.plan.desc', hintKey: 'chat.command.plan.hint', icon: ClipboardList },
  { cmd: '/clear', descKey: 'chat.command.clear.desc', hintKey: 'chat.command.clear.hint', icon: Eraser },
  { cmd: '/compact', descKey: 'chat.command.compact.desc', hintKey: 'chat.command.compact.hint', icon: Archive },
  { cmd: '/status', descKey: 'chat.command.status.desc', hintKey: 'chat.command.status.hint', icon: Sparkles },
  { cmd: '/help', descKey: 'chat.command.help.desc', hintKey: 'chat.command.help.hint', icon: BadgeHelp },
] as const

export interface SkillCommand {
  name: string
  description: string
}

export type CommandScope = 'all' | 'skills' | 'none'
export type BuiltinCommand = typeof COMMANDS[number]['cmd']

/** Builds one ordered, indexed suggestion list; prompt names never become executable commands. */
export function useInputCommands({ scope, builtinCommands, skills, quickPrompts, goalEnabled, draftKey }: {
  scope: CommandScope
  builtinCommands?: BuiltinCommand[]
  skills: SkillCommand[]
  quickPrompts: AgentQuickPromptSettings[]
  goalEnabled: boolean
  draftKey?: string
}) {
  const { t } = useTranslation()
  const [showCommands, setShowCommands] = useState(false)
  const [commandQuery, setCommandQuery] = useState<string | null>(null)
  const [activeCommandIndex, setActiveCommandIndex] = useState(0)
  useEffect(() => {
    setShowCommands(false)
    setCommandQuery(null)
    setActiveCommandIndex(0)
  }, [draftKey])
  const menu = useMemo(() => {
    const allowed = builtinCommands ? new Set<string>(builtinCommands) : null
    const builtins: InputCommandOption[] = scope === 'all' ? COMMANDS
      .filter(({ cmd }) => cmd !== '/goal' || goalEnabled)
      .filter(({ cmd }) => !allowed || allowed.has(cmd))
      .map(({ cmd, descKey, hintKey, icon }) => ({
        cmd, description: t(descKey), hint: t(hintKey), icon, source: 'builtin',
      })) : []
    const seen = new Set(builtins.map((command) => command.cmd))
    const skillOptions: InputCommandOption[] = skills
      .map((skill) => ({
        cmd: `/${skill.name}`,
        description: skill.description || skill.name,
        hint: t('chat.command.skill.hint'),
        icon: Sparkles,
        source: 'skill' as const,
      }))
      .filter(({ cmd }) => {
        if (seen.has(cmd)) return false
        seen.add(cmd)
        return true
      })
    const prompts: InputCommandOption[] = scope === 'all' ? quickPrompts
      .filter((prompt) => prompt.enabled)
      .map((prompt) => ({
        cmd: prompt.name,
        id: prompt.id,
        prompt: prompt.prompt,
        description: '',
        hint: t('chat.quick.behavior.fill'),
        icon: Sparkles,
        source: 'quick-prompt',
      })) : []
    const commands = scope === 'none' ? [] : [...builtins, ...prompts, ...skillOptions]
    const normalizedQuery = commandQuery?.toLocaleLowerCase() ?? ''
    const filteredCommands = commandQuery === null ? [] : commands.filter((command) => command.source === 'quick-prompt'
      ? command.cmd.toLocaleLowerCase().includes(normalizedQuery)
      : command.cmd.toLocaleLowerCase().startsWith(`/${normalizedQuery}`))
    const indexed = filteredCommands.map((command, index) => ({ command, index }))
    return {
      filteredCommands,
      filteredBuiltinCommands: indexed.filter(({ command }) => command.source === 'builtin'),
      filteredQuickPromptCommands: indexed.filter(({ command }) => command.source === 'quick-prompt'),
      filteredSkillCommands: indexed.filter(({ command }) => command.source === 'skill'),
    }
  }, [builtinCommands, goalEnabled, commandQuery, quickPrompts, scope, skills, t])
  useEffect(() => {
    if (activeCommandIndex >= menu.filteredCommands.length) setActiveCommandIndex(0)
  }, [activeCommandIndex, menu.filteredCommands.length])
  return { ...menu, showCommands, setShowCommands, setCommandQuery, activeCommandIndex, setActiveCommandIndex }
}
