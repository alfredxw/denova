import { motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { novaEase } from '@/features/motion/motion-tokens'
import type { ActorStateInitialActor, ActorStateTemplate } from '../../../../types'
import type { ExplorerProps } from '../types'
import { actorNodeId } from '../build-tree'
import { TemplateStateEditor } from '../shared/TemplateStateEditor'
import { DetailResponsiveGrid } from './DetailLayout'

interface ActorDetailEditorProps {
  actor: ActorStateInitialActor
  actorIndex: number
  template?: ActorStateTemplate
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
  onIdChange: (nextId: string) => void
}

export function ActorDetailEditor({
  actor,
  actorIndex,
  template,
  value,
  onChange,
  onIdChange,
}: ActorDetailEditorProps) {
  const { t } = useTranslation()
  const templates = value.templates || []

  const updateActor = (patch: Partial<ActorStateInitialActor>) => {
    const actors = [...(value.initial_actors || [])]
    const nextActor = { ...actor, ...patch }
    actors[actorIndex] = nextActor
    const nextNodeId = actorNodeId(nextActor, actorIndex)
    if (nextNodeId !== actorNodeId(actor, actorIndex)) onIdChange(nextNodeId)
    onChange({ ...value, initial_actors: actors })
  }

  const handleTemplateChange = (templateId: string) => {
    updateActor({ template_id: templateId })
  }

  const handleStateChange = (state: Record<string, unknown>) => {
    updateActor({ state })
  }

  return (
    <DetailResponsiveGrid className="items-start">
      {/* Basic info */}
      <motion.section
        initial={{ opacity: 0, y: 4 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.2, ease: novaEase }}
        className="space-y-3"
      >
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.actorInfo')}
          </span>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <FormField label="ID">
            <Input
              className="nova-field h-8 text-xs focus-visible:ring-0"
              value={actor.id}
              onChange={(e) => updateActor({ id: e.target.value })}
            />
          </FormField>
          <FormField label={t('settingPanel.field.name')}>
            <Input
              className="nova-field h-8 text-xs focus-visible:ring-0"
              value={actor.name || ''}
              onChange={(e) => updateActor({ name: e.target.value })}
              placeholder={t('settingPanel.actorState.explorer.actorNamePlaceholder')}
            />
          </FormField>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <FormField label={t('settingPanel.actorState.template')}>
            <Select
              value={actor.template_id}
              onValueChange={handleTemplateChange}
            >
              <SelectTrigger className="nova-field h-8 text-xs focus:ring-0">
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="nova-panel border text-[var(--nova-text)]">
                <SelectGroup>
                  {templates.map((item) => (
                    <SelectItem key={item.id} value={item.id}>{item.name || item.id}</SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label={t('settingPanel.actorState.role')}>
            <Input
              className="nova-field h-8 text-xs focus-visible:ring-0"
              value={actor.role || ''}
              onChange={(e) => updateActor({ role: e.target.value })}
              placeholder="protagonist / supporting / ..."
            />
          </FormField>
        </div>

        <FormField label={t('common.description')}>
          <Textarea
            className="nova-field min-h-[48px] resize-none text-xs focus-visible:ring-0"
            value={actor.description || ''}
            onChange={(e) => updateActor({ description: e.target.value })}
            placeholder={t('settingPanel.actorState.explorer.actorDescriptionPlaceholder')}
          />
        </FormField>
      </motion.section>

      {/* Initial state */}
      <section className="space-y-2">
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.initialState')}
          </span>
          {template ? (
            <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
              {template.name || template.id}
            </span>
          ) : null}
        </div>
        {template ? (
          <TemplateStateEditor
            template={template}
            state={actor.state || {}}
            onChange={handleStateChange}
          />
        ) : (
          <div className="rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-4 text-center text-[11px] text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.chooseTemplateFirst')}
          </div>
        )}
      </section>
    </DetailResponsiveGrid>
  )
}

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <label className="text-[11px] text-[var(--nova-text-faint)]">{label}</label>
      {children}
    </div>
  )
}
