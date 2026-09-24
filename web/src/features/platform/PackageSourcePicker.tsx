import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldLabel, FieldGroup, FieldDescription } from '@/components/ui/field'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { requestJSON } from '@/lib/api-client/client'
import { notifyAgentChatProjectUpdated } from '@/features/agent-chat/api'
import { management, managementBase, platformError, type Candidate, type DevelopmentSource } from './api'
import { openExtensionSource } from './extension-navigation'

/** Installation sources produce the same candidate; importing explicitly creates a source Project. */
export function PackageSourcePicker({ busy, run, onPreview, onImported }: {
  busy: boolean
  run: (action: () => Promise<void>) => Promise<void>
  onPreview: (load: () => Promise<Candidate>) => Promise<void>
  onImported: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [url, setURL] = useState('')
  const [ref, setRef] = useState('')
  const [path, setPath] = useState('')
  const [directory, setDirectory] = useState('')
  const [error, setError] = useState('')
  const source = { url: url.trim(), ref: ref.trim(), path: path.trim() }
  const github = (action: () => Promise<void>) => run(async () => {
    setError('')
    try { await action() } catch (error) { setError(platformError(error)); throw error }
  })
  return <Tabs defaultValue="github">
    <TabsList className="w-full" aria-label={t('platform.github.installSource')}>
      <TabsTrigger value="github" disabled={busy}>GitHub</TabsTrigger>
      <TabsTrigger value="local" disabled={busy}>{t('platform.github.local')}</TabsTrigger>
    </TabsList>
    <TabsContent value="github">
      <FieldGroup className="pt-3">
        <Field>
          <FieldLabel htmlFor="package-github-url">{t('platform.github.repository')}</FieldLabel>
          <Input id="package-github-url" type="url" value={url} disabled={busy} placeholder="https://github.com/author/repository" onChange={event => { setURL(event.target.value); setError('') }} />
          <FieldDescription>{t('platform.github.help')}</FieldDescription>
        </Field>
        <details>
          <summary className="cursor-pointer text-sm text-muted-foreground">{t('platform.advancedDevelopment')}</summary>
          <FieldGroup className="mt-3">
            <Field>
              <FieldLabel htmlFor="package-github-ref">{t('platform.github.ref')}</FieldLabel>
              <Input id="package-github-ref" value={ref} disabled={busy} placeholder={t('platform.github.defaultBranch')} onChange={event => setRef(event.target.value)} />
            </Field>
            <Field>
              <FieldLabel htmlFor="package-github-path">{t('platform.github.path')}</FieldLabel>
              <Input id="package-github-path" value={path} disabled={busy} placeholder={t('platform.github.root')} onChange={event => setPath(event.target.value)} />
            </Field>
          </FieldGroup>
        </details>
        {error && <InlineErrorNotice message={error} />}
        <div className="flex flex-wrap gap-2">
          <Button disabled={busy || !source.url} onClick={() => void github(() => onPreview(() => management('/packages/github/preview', 'POST', source)))}>{t(busy ? 'platform.github.loading' : 'platform.github.checkPackage')}</Button>
          <Button variant="outline" disabled={busy || !source.url} onClick={() => void github(async () => {
            const imported = await management<DevelopmentSource>('/packages/github/import', 'POST', source)
            notifyAgentChatProjectUpdated(imported.projectId)
            await client.invalidateQueries({ queryKey: ['platform', 'development'] })
            await openExtensionSource(imported)
            onImported()
          })}>{t('platform.github.importSource')}</Button>
        </div>
        <p className="text-sm text-muted-foreground">{t('platform.github.buildHelp')}</p>
      </FieldGroup>
    </TabsContent>
    <TabsContent value="local">
      <FieldGroup className="pt-3">
        <Field>
          <FieldLabel htmlFor="package-directory">{t('platform.directory')}</FieldLabel>
          <Input id="package-directory" value={directory} disabled={busy} onChange={event => setDirectory(event.target.value)} />
        </Field>
        <Button disabled={busy || !directory.trim()} variant="outline" onClick={() => void run(() => onPreview(() => management('/packages/preview', 'POST', { directory })))}>{t('platform.check')}</Button>
        <Field>
          <FieldLabel htmlFor="package-archive">{t('platform.archive')}</FieldLabel>
          <Input id="package-archive" type="file" accept=".zip" disabled={busy} onChange={event => {
            const file = event.target.files?.[0]
            if (file) void run(() => onPreview(() => requestJSON(`${managementBase}/packages/preview`, {
              method: 'POST', headers: { 'Content-Type': 'application/zip' }, body: file,
            })))
          }} />
        </Field>
      </FieldGroup>
    </TabsContent>
  </Tabs>
}
