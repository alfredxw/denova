export interface StoryStageCommandItem {
  name: string
  description: string
  hint: string
  builtIn: boolean
}

interface SkillCommand {
  name: string
  description?: string
}

interface CommandLabels {
  compactDescription: string
  compactHint: string
  skillHint: string
}

interface StoryStageCommandMenu {
  commands: StoryStageCommandItem[]
  builtInItems: Array<{ command: StoryStageCommandItem; index: number }>
  skillItems: Array<{ command: StoryStageCommandItem; index: number }>
}

export function buildStoryStageCommandMenu(query: string | null, skills: SkillCommand[], labels: CommandLabels): StoryStageCommandMenu {
  if (query === null) return { commands: [], builtInItems: [], skillItems: [] }
  const normalizedQuery = query.toLowerCase()
  const seen = new Set(['compact'])
  const commands: StoryStageCommandItem[] = [{
    name: 'compact',
    description: labels.compactDescription,
    hint: labels.compactHint,
    builtIn: true,
  }]

  for (const skill of skills) {
    if (seen.has(skill.name)) continue
    seen.add(skill.name)
    commands.push({
      name: skill.name,
      description: skill.description || skill.name,
      hint: labels.skillHint,
      builtIn: false,
    })
  }

  const filtered = commands.filter((command) => command.name.toLowerCase().startsWith(normalizedQuery))
  const indexed = filtered.map((command, index) => ({ command, index }))
  return {
    commands: filtered,
    builtInItems: indexed.filter(({ command }) => command.builtIn),
    skillItems: indexed.filter(({ command }) => !command.builtIn),
  }
}
