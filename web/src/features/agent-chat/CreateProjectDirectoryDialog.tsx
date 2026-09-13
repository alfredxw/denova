import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Gamepad2, Puzzle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldGroup, FieldLabel, FieldTitle } from '@/components/ui/field'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { jsonHeaders, requestJSON } from '@/lib/api-client/client'
import { useWorkspaceStore } from '@/stores/workspace-store'
import { management, platformError, type PackageKind } from '@/features/platform/api'
import { createAgentChatSession, notifyAgentChatProjectUpdated } from './api'
import { requestAgentChatSessionNavigation } from './session-navigation'
import { writeAgentChatActiveSession } from './session-preferences'

/** Both entry points create ordinary Projects. Templates only initialize their source files. */
export function CreateProjectDirectoryDialog({ open, onOpenChange, extension = false, initialKind = 'plugin' }: {
  open: boolean
  onOpenChange: (open: boolean) => void
  extension?: boolean
  /** Seeds a new extension form; category entry points mount a fresh dialog. */
  initialKind?: PackageKind
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [parent, setParent] = useState('')
  const initialTemplate = extension ? (initialKind === 'plugin' ? 'http-tool' : 'npc-game') : 'empty'
  const [template, setTemplate] = useState(initialTemplate)
  const [packageId, setPackageId] = useState('')
  const [instruction, setInstruction] = useState('')
  const [busy, setBusy] = useState(false)
  const created = useRef<{ id: string; hasGuide: boolean; sessionId?: string } | null>(null)
  useEffect(() => {
    if (open) return
    created.current = null
    setName(''); setParent(''); setPackageId(''); setInstruction('')
    setTemplate(initialTemplate)
  }, [open, initialTemplate])
  const create = async () => {
    setBusy(true)
    try {
      let project = created.current
      if (!project) {
        const record = await requestJSON<{ id: string }>('/api/agent-chat/projects/directory', {
          method: 'POST', headers: jsonHeaders,
          body: JSON.stringify({ name: name.trim(), parent_directory: parent.trim() }),
        })
        project = { id: record.id, hasGuide: false }
        created.current = project
        if (template !== 'empty') {
          try {
            await management('/development', 'POST', {
              kind: template === 'http-tool' ? 'plugin' : 'game', projectId: project.id,
              relativePath: '.', templateId: template, id: packageId.trim() || `extension.${crypto.randomUUID()}`,
              name: { 'zh-CN': name.trim(), 'en-US': name.trim() },
            })
            project.hasGuide = true
          } catch (error) {
            console.error('[create-project] Template initialization failed; retaining the new Project for repair', { projectId: project.id, error })
            toast.error(t('platform.sourceInitFailed'))
          }
        }
      }
      if (!project.sessionId) {
        const session = await createAgentChatSession(project.id, template === 'empty' ? '' : t('platform.developConversation', { name: name.trim() }))
        project.sessionId = session.id
      }
      await queryClient.invalidateQueries({ queryKey: ['platform'] })
      notifyAgentChatProjectUpdated(project.id)
      writeAgentChatActiveSession(project.id, project.sessionId)
      requestAgentChatSessionNavigation({
        projectId: project.id, sessionId: project.sessionId,
        ...(project.hasGuide ? { sourcePath: 'DEVELOPMENT.md', initialInstruction: instruction.trim() } : {}),
      })
      useWorkspaceStore.getState().setMode('agentchat')
      onOpenChange(false)
    } catch (error) { toast.error(platformError(error)) }
    finally { setBusy(false) }
  }
  return (
    <Dialog open={open} onOpenChange={next => { if (!busy) onOpenChange(next) }}>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t(extension ? 'platform.createExtension' : 'platform.createDirectory')}</DialogTitle>
          <DialogDescription>{t(extension ? 'platform.createDescription' : 'platform.createDirectoryDescription')}</DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="new-project-name">{t(extension ? 'platform.extensionName' : 'platform.projectName')}</FieldLabel>
            <Input id="new-project-name" autoFocus value={name} disabled={busy || !!created.current} onChange={event => setName(event.target.value)} />
          </Field>
          {extension && <>
            <Field data-disabled={busy || !!created.current}>
              <FieldTitle id="new-extension-kind-label">{t('platform.extensionType')}</FieldTitle>
              <ToggleGroup type="single" variant="outline" size="lg" className="w-full" aria-labelledby="new-extension-kind-label"
                value={template} onValueChange={value => { if (value) setTemplate(value) }} disabled={busy || !!created.current}>
                <ToggleGroupItem value="http-tool" className="min-w-0 flex-1" title={t('platform.developPlugin')}><Puzzle data-icon="inline-start" />{t('platform.type.plugin')}</ToggleGroupItem>
                <ToggleGroupItem value="npc-game" className="min-w-0 flex-1" title={t('platform.developGame')}><Gamepad2 data-icon="inline-start" />{t('platform.type.game')}</ToggleGroupItem>
              </ToggleGroup>
            </Field>
            <Field>
              <FieldLabel htmlFor="new-extension-instruction">{t('platform.idea')}</FieldLabel>
              <Textarea id="new-extension-instruction" value={instruction} maxLength={16000} disabled={busy} placeholder={t('platform.ideaPlaceholder')} onChange={event => setInstruction(event.target.value)} />
            </Field>
          </>}
          <details open={!extension}>
            <summary className="cursor-pointer text-sm text-muted-foreground">{t('platform.advancedDevelopment')}</summary>
          <FieldGroup className="mt-3">
          {!extension && <Field>
            <FieldLabel htmlFor="new-project-template">{t('platform.template')}</FieldLabel>
            <Select value={template} onValueChange={setTemplate} disabled={busy || !!created.current}>
              <SelectTrigger id="new-project-template" className="w-full"><SelectValue /></SelectTrigger>
              <SelectContent><SelectGroup>{['empty', 'http-tool', 'npc-game'].map(value => (
                <SelectItem key={value} value={value}>{t('platform.template.' + value)}</SelectItem>
              ))}</SelectGroup></SelectContent>
            </Select>
          </Field>}
          {template !== 'empty' && <Field>
            <FieldLabel htmlFor="new-project-package">{t('platform.packageId')}</FieldLabel>
            <Input id="new-project-package" value={packageId} placeholder={t('platform.automaticId')} disabled={busy || !!created.current} onChange={event => setPackageId(event.target.value)} />
          </Field>}
          <details>
            <summary className="cursor-pointer text-sm text-muted-foreground">{t('platform.directoryLocation')}</summary>
            <Field className="mt-3">
              <FieldLabel htmlFor="new-project-parent">{t('platform.parentDirectory')}</FieldLabel>
              <Input id="new-project-parent" value={parent} placeholder={t('platform.managedDirectory')} disabled={busy || !!created.current} onChange={event => setParent(event.target.value)} />
            </Field>
          </details>
          </FieldGroup>
          </details>
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={busy || !name.trim()} onClick={() => void create()}>{t('platform.createAndDevelop')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
