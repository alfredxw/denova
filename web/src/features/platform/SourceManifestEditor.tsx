import { useEffect, useId, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Field, FieldGroup, FieldLabel, FieldDescription } from '@/components/ui/field'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { useProjectFileEditor } from '@/features/files/use-project-file-editor'
import { sourceManifestPath } from './extension-directory'
import type { DevelopmentSource } from './api'

// Only the fields owned by this form are required; all other source fields survive edits.
const editableManifest = z.object({
  id: z.string(), version: z.string(),
  name: z.object({ 'zh-CN': z.string(), 'en-US': z.string() }).passthrough(),
  description: z.object({ 'zh-CN': z.string().optional(), 'en-US': z.string().optional() }).passthrough().optional(),
  permissions: z.object({ required: z.array(z.string()).optional(), optional: z.array(z.string()).optional() }).passthrough().optional(),
  game: z.object({ cover: z.string().optional() }).passthrough().optional(),
}).passthrough()

export function SourceManifestEditor({ source, refreshSignal, onSaved, onOpenFile, onFlushChange }: {
  source: DevelopmentSource
  refreshSignal: number
  onSaved: () => void
  onOpenFile: (path: string) => void
  onFlushChange: (flush: EditorFlushHandler | null) => void
}) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const path = sourceManifestPath(source)
  const editor = useProjectFileEditor({ projectId: source.projectId, selectedPath: path, autoSaveEnabled: true, autoSaveDelayMs: 1200, onSaved })
  const language = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US'
  const refreshed = useRef(refreshSignal)
  const manifest = useMemo(() => {
    try { return editableManifest.parse(JSON.parse(editor.draft)) } catch { return null }
  }, [editor.draft])
  useEffect(() => { onFlushChange(editor.flush); return () => onFlushChange(null) }, [editor.flush, onFlushChange])
  useEffect(() => {
    // Finish the initial file read before asking the shared editor to rebase it.
    if (editor.loading || !editor.document || refreshed.current === refreshSignal) return
    refreshed.current = refreshSignal
    void editor.reload().catch(error => console.error('[extensions] refresh manifest failed', { projectId: source.projectId, error }))
  }, [editor.loading, editor.document, editor.reload, refreshSignal, source.projectId])
  const updateManifest = (changes: Partial<z.infer<typeof editableManifest>>) => {
    editor.setDraft(JSON.stringify({ ...manifest, ...changes }, null, 2) + '\n')
  }
  if (editor.loading || !editor.document && !editor.error) return <p role="status" className="p-6 text-sm text-muted-foreground">{t('common.loading')}</p>
  return <div className="flex min-w-0 flex-col gap-4">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <AutosaveStatusIndicator status={editor.status} error={editor.autoSaveError ? t('platform.sourceSaveFailed') : null} onRetry={() => editor.retry()} />
    </div>
    {editor.error && <InlineErrorNotice message={t('platform.sourceSaveFailed')} />}
    {!manifest ? <InlineErrorNotice message={t('platform.sourceInvalid')} /> : <FieldGroup>
      <Field><FieldLabel htmlFor={id + '-name'}>{t('platform.extensionName')}</FieldLabel>
        <Input id={id + '-name'} value={manifest.name[language]} onChange={event => updateManifest({ name: { ...manifest.name, [language]: event.target.value } })} />
      </Field>
      <Field><FieldLabel htmlFor={id + '-description'}>{t('platform.extensionDescription')}</FieldLabel>
        <Textarea id={id + '-description'} value={manifest.description?.[language] ?? ''} onChange={event => updateManifest({ description: { ...manifest.description, [language]: event.target.value } })} />
        <FieldDescription>{t('platform.localizedSourceHelp')}</FieldDescription>
      </Field>
      {source.kind === 'game' && manifest.game && <Field><FieldLabel htmlFor={id + '-cover'}>{t('platform.sourceCover')}</FieldLabel>
        <Input id={id + '-cover'} value={manifest.game.cover ?? ''} placeholder="cover.png" onChange={event => updateManifest({ game: { ...manifest.game, cover: event.target.value || undefined } })} />
        <FieldDescription>{t('platform.sourceCoverHelp')}</FieldDescription>
      </Field>}
      <Field><FieldLabel htmlFor={id + '-version'}>{t('platform.sourceVersion')}</FieldLabel>
        <Input id={id + '-version'} value={manifest.version} onChange={event => updateManifest({ version: event.target.value })} />
        <FieldDescription>{t('platform.versionHelp')}</FieldDescription>
      </Field>
    </FieldGroup>}
    <details>
      <summary className="cursor-pointer text-sm text-muted-foreground">{t('platform.packageDetails')}</summary>
      <div className="mt-3 flex min-w-0 flex-col gap-4">
        {manifest && <FieldGroup>
          <Field><FieldLabel htmlFor={id + '-id'}>{t('platform.packageId')}</FieldLabel><Input id={id + '-id'} value={manifest.id} readOnly /><FieldDescription>{t('platform.identityHelp')}</FieldDescription></Field>
          <Field><FieldLabel>{t('platform.capabilities')}</FieldLabel>
            <div className="flex flex-wrap gap-2">{[...(manifest.permissions?.required ?? []), ...(manifest.permissions?.optional ?? [])].map(permission => <Badge key={permission} variant="secondary">{t('platform.permission.' + permission, { defaultValue: permission })}</Badge>)}</div>
            <FieldDescription>{t('platform.capabilitiesHelp')}</FieldDescription>
          </Field>
        </FieldGroup>}
        <Button variant="outline" className="self-start" onClick={() => onOpenFile(path)}>{t('platform.openManifest')}</Button>
      </div>
    </details>
  </div>
}
