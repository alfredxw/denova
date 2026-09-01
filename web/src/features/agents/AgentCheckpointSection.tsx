import type { ReactElement } from 'react'
import type { TFunction } from 'i18next'
import { ScrollText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Textarea } from '@/components/ui/textarea'
import type { AgentContextOverride, AgentPromptSource, ResolvedAgentContextSettings } from '@/features/settings/types'
import { Field, SectionTitle } from './agent-form-controls'
import { ReadonlyPromptSourceBlock } from './readonly-prompt-source-block'

const checkpointGuidanceMaxCharacters = 1000
const checkpointGuidanceMaxCodeUnits = checkpointGuidanceMaxCharacters * 2

interface AgentCheckpointSectionProps {
  value: AgentContextOverride
  resolved: ResolvedAgentContextSettings
  sources?: AgentPromptSource[]
  onChange: (patch: Partial<AgentContextOverride>) => void
}

export function AgentCheckpointSection({ value, resolved, sources = [], onChange }: AgentCheckpointSectionProps): ReactElement {
  const { t } = useTranslation()
  const overridden = value.checkpoint_guidance != null
  const guidance = value.checkpoint_guidance ?? resolved.checkpoint_guidance ?? ''
  const guidanceLength = Array.from(guidance).length

  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={ScrollText} title={t('agents.section.checkpoint')} />
      <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agents.checkpoint.protocolNote')}</p>
      <div className="flex flex-col gap-2">
        {sources.map((source) => (
          <ReadonlyPromptSourceBlock
            key={source.id}
            title={compactionSourceTitle(t, source)}
            source={source.source}
            content={source.content}
          />
        ))}
      </div>
      <Field
        label={t('agents.field.checkpointGuidance')}
        inherited={!overridden}
        onReset={overridden ? () => onChange({ checkpoint_guidance: null }) : undefined}
      >
        <Textarea
          autoResize
          maxLength={checkpointGuidanceMaxCodeUnits}
          value={guidance}
          aria-label={t('agents.field.checkpointGuidance')}
          placeholder={t('agents.checkpoint.guidancePlaceholder')}
          onChange={(event) => onChange({ checkpoint_guidance: truncateCheckpointGuidance(event.target.value) })}
          className="min-h-32 min-w-0 flex-1 resize-y font-mono text-xs leading-5"
        />
      </Field>
      <div className="flex items-start justify-between gap-3 text-[10px] leading-4 text-[var(--nova-text-faint)]">
        <span>{t('agents.checkpoint.guidanceNote')}</span>
        <span className="shrink-0">{guidanceLength}/{checkpointGuidanceMaxCharacters}</span>
      </div>
    </section>
  )
}

function compactionSourceTitle(t: TFunction, source: AgentPromptSource): string {
  const key = `agents.checkpoint.source.${source.id}`
  const translated = t(key)
  return translated === key ? source.title : translated
}

function truncateCheckpointGuidance(value: string): string {
  const characters = Array.from(value)
  if (characters.length <= checkpointGuidanceMaxCharacters) return value
  return characters.slice(0, checkpointGuidanceMaxCharacters).join('')
}
