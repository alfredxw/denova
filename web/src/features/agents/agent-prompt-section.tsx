import { ChevronDown, ScrollText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Textarea } from '@/components/ui/textarea'
import type { AgentPromptBlocks, AgentPromptOverride, AgentPromptSource } from '@/features/settings/types'
import { AgentConfigurationDisclosure } from './agent-configuration-disclosure'
import { Field } from './agent-form-controls'

interface PromptEditorProps {
  value: AgentPromptOverride
  inherited: AgentPromptOverride
  onChange: (patch: Partial<AgentPromptOverride>) => void
}

/** Keep personal instructions first; preserve the source order and exact prompt text in references. */
export function AgentPromptSection({ value, inherited, builtin, blocks, sources, onChange }: PromptEditorProps & {
  builtin: string
  blocks?: AgentPromptBlocks
  sources?: AgentPromptSource[]
}) {
  const { t } = useTranslation()
  const promptSources = sources?.length ? sources : fallbackPromptSources(blocks, builtin)
  const custom = promptSources.find((source) => source.editable && source.field === 'system_prompt')
  const references = promptSources.filter((source) => source !== custom)
  const editor = { value, inherited, onChange }
  return (
    <section className="flex min-w-0 flex-col gap-6">
      {custom && <PromptEditor source={custom} {...editor} />}
      <AgentConfigurationDisclosure id="prompt-sources" icon={ScrollText} headingLevel={3}
        title={t('agents.prompt.references')} summary={t('agents.prompt.referencesSummary')}>
        <div className="divide-y divide-[var(--nova-border)]">
          {references.map((source) => <PromptReference key={`${source.id}:${source.field ?? 'readonly'}`} source={source} {...editor} />)}
        </div>
      </AgentConfigurationDisclosure>
    </section>
  )
}

function PromptEditor({ source, value, inherited, onChange }: PromptEditorProps & { source: AgentPromptSource }) {
  const { t } = useTranslation()
  const field = source.field!
  const hasOverride = Boolean(value[field]?.trim())
  const content = hasOverride ? value[field]! : inherited[field]?.trim() ? inherited[field]! : source.content ?? ''
  const title = promptSourceTitle(t, source)
  return <Field label={title} inherited={!hasOverride} onReset={hasOverride ? () => onChange({ [field]: '' }) : undefined}>
    <Textarea value={content} aria-label={title}
      placeholder={t(field === 'system_prompt' ? 'agents.prompt.customPlaceholder' : 'agents.prompt.placeholder')}
      onChange={(event) => onChange({ [field]: event.target.value })}
      className="min-h-48 resize-y rounded-[var(--nova-radius)] text-sm leading-relaxed" />
  </Field>
}

function PromptReference({ source, ...editor }: PromptEditorProps & { source: AgentPromptSource }) {
  const { t } = useTranslation()
  const editable = Boolean(source.editable && source.field)
  const customized = editable && Boolean(editor.value[source.field!]?.trim())
  const title = promptSourceTitle(t, source)
  return <Collapsible className="min-w-0 py-2">
    <h4>
      <CollapsibleTrigger className="group flex min-h-11 w-full items-center gap-3 rounded-[var(--nova-radius)] text-left hover:bg-[var(--nova-hover)] focus-visible:outline-2 focus-visible:outline-ring">
        <ChevronDown className="size-4 shrink-0 text-muted-foreground transition-transform group-data-[state=open]:rotate-180 motion-reduce:transition-none" />
        <span className="min-w-0 flex-1 break-words text-xs font-medium">{title}</span>
        <span className="shrink-0 text-xs font-normal text-muted-foreground">{t(customized ? 'agents.badge.customized' : editable ? 'agents.prompt.badge.editable' : 'agents.prompt.badge.readonly')}</span>
      </CollapsibleTrigger>
    </h4>
    <CollapsibleContent className="min-w-0 pb-3 pt-2">
      {source.id === 'teller' && source.source && <p className="mb-2 text-xs text-muted-foreground">{source.source}</p>}
      {editable ? <PromptEditor source={source} {...editor} /> : source.content?.trim()
        ? <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words text-xs leading-relaxed text-muted-foreground">{source.content}</pre>
        : <p className="text-xs text-muted-foreground">{t('agents.prompt.empty')}</p>}
    </CollapsibleContent>
  </Collapsible>
}

function fallbackPromptSources(blocks?: AgentPromptBlocks, builtin?: string): AgentPromptSource[] {
  return [
    ...(blocks?.runtime_contract ? [{ id: 'runtime_contract', title: 'Runtime Contract', source: 'Denova runtime', content: blocks.runtime_contract }] : []),
    ...(blocks?.output_protocol ? [{ id: 'output_protocol', title: 'Output Format', source: 'Denova runtime', content: blocks.output_protocol }] : []),
    { id: 'flow', title: 'Flow Rules', source: 'Denova built-in', content: blocks?.editable_system_prompt || builtin || '', editable: true, field: 'flow_prompt' },
    { id: 'custom', title: 'Custom Rules', source: 'user/workspace config', content: '', editable: true, field: 'system_prompt' },
  ]
}

function promptSourceTitle(t: ReturnType<typeof useTranslation>['t'], source: AgentPromptSource) {
  return t(`agents.prompt.source.${source.id}`, { defaultValue: source.title })
}
