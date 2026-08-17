import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface NewScriptToolDialogProps {
  open: boolean
  existingPaths: string[]
  onOpenChange: (open: boolean) => void
  onCreate: (file: { path: string; content: string }) => void
}

const targets = ['general', 'ide', 'interactive_story'] as const
const namePattern = /^[a-z][a-z0-9_-]{0,63}$/

export function NewScriptToolDialog({ open, existingPaths, onOpenChange, onCreate }: NewScriptToolDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [inputFields, setInputFields] = useState('')
  const [agents, setAgents] = useState<Set<string>>(() => new Set(['general', 'ide']))
  useEffect(() => {
    if (!open) return
    setName('')
    setDescription('')
    setInputFields('')
    setAgents(new Set(['general', 'ide']))
  }, [open])
  const fields = useMemo(() => normalizedInputFields(inputFields), [inputFields])
  const normalizedName = name.trim()
  const path = `tools/${normalizedName}.js`
  let error = ''
  if (!namePattern.test(normalizedName)) error = t('continualLearning.script.nameError')
  else if (existingPaths.includes(path)) error = t('continualLearning.script.nameConflict')
  else if (!description.trim()) error = t('continualLearning.script.descriptionRequired')
  else if (agents.size === 0) error = t('continualLearning.script.agentRequired')
  else if (fields.invalid.length > 0) {
    error = t('continualLearning.script.inputError', { fields: fields.invalid.join(', ') })
  }

  const toggleAgent = (agent: string) => {
    setAgents(current => {
      const next = new Set(current)
      if (next.has(agent)) next.delete(agent)
      else next.add(agent)
      return next
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('continualLearning.script.new')}</DialogTitle>
          <DialogDescription>{t('continualLearning.script.newDescription')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="script-tool-name">{t('continualLearning.script.name')}</Label>
            <Input id="script-tool-name" value={name} onChange={event => setName(event.target.value)} placeholder="research_company" autoFocus />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="script-tool-description">{t('continualLearning.script.description')}</Label>
            <Input id="script-tool-description" value={description} onChange={event => setDescription(event.target.value)} placeholder="Research a company and return a concise brief." />
            <p className="text-[11px] text-[var(--nova-text-faint)]">{t('continualLearning.script.modelVisible')}</p>
          </div>
          <div className="grid gap-1.5">
            <Label>{t('continualLearning.script.agents')}</Label>
            <div className="flex flex-wrap gap-2">
              {targets.map(agent => (
                <Button key={agent} type="button" size="xs" variant={agents.has(agent) ? 'default' : 'outline'} onClick={() => toggleAgent(agent)}>
                  {t(`continualLearning.script.agent.${agent}`)}
                </Button>
              ))}
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="script-tool-inputs">{t('continualLearning.script.inputs')}</Label>
            <Input id="script-tool-inputs" value={inputFields} onChange={event => setInputFields(event.target.value)} placeholder="company, question" />
            <p className="text-[11px] text-[var(--nova-text-faint)]">{t('continualLearning.script.inputsHint')}</p>
          </div>
          {error ? <p role="alert" className="text-xs text-[var(--nova-danger)]">{error}</p> : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button
            type="button"
            disabled={Boolean(error)}
            onClick={() => {
              onCreate({ path, content: scriptToolTemplate(normalizedName, description.trim(), [...agents], fields.valid) })
              onOpenChange(false)
            }}
          >
            {t('common.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function normalizedInputFields(value: string) {
  const fields = [...new Set(value.split(',').map(item => item.trim()).filter(Boolean))]
  return {
    valid: fields.filter(field => namePattern.test(field)),
    invalid: fields.filter(field => !namePattern.test(field)),
  }
}

function scriptToolTemplate(name: string, description: string, agents: string[], fields: string[]) {
  const required = fields.length ? `\n  required: [${fields.join(', ')}]` : ''
  const properties = fields.length
    ? `\n  properties:\n${fields.map(field => `    ${field}:\n      type: string\n      minLength: 1\n      description: Input value for ${field}.`).join('\n')}`
    : '\n  properties: {}'
  return `---
name: ${name}
description: ${JSON.stringify(description)}
agents: [${agents.join(', ')}]
enabled: true
input_schema:
  type: object
  additionalProperties: false${required}${properties}
---
// Compose the tools available to this Agent through ctx.tools.call or ctx.tools.parallel.
return input
`
}
