import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { importExtensionSource } from '@/features/platform/extension-navigation'

export type ResourceInput =
  | { kind: 'github' | 'https_zip'; url: string; ref?: string; path?: string }
  | { kind: 'file'; file?: File }
  | { kind: 'directory'; directory: string }

/** Source selection is shared by resource bundles and standalone extensions. */
export function ImportSourceFields({ value, onChange, busy, run, onImportedSource }: {
  value: ResourceInput
  onChange: (value: ResourceInput) => void
  busy: boolean
  run: (action: () => Promise<void>) => Promise<void>
  onImportedSource: () => void
}) {
  const { t } = useTranslation()
  return <FieldGroup>
    <Field>
      <FieldLabel htmlFor="resource-source-kind">{t('market.import.source')}</FieldLabel>
      <Select value={value.kind} disabled={busy} onValueChange={kind => {
        switch (kind) {
          case 'github': case 'https_zip': onChange({ kind, url: '' }); break
          case 'file': onChange({ kind }); break
          case 'directory': onChange({ kind, directory: '' }); break
        }
      }}>
        <SelectTrigger id="resource-source-kind" className="w-full"><SelectValue /></SelectTrigger>
        <SelectContent><SelectGroup>
          <SelectItem value="github">GitHub</SelectItem>
          <SelectItem value="https_zip">{t('market.import.url')}</SelectItem>
          <SelectItem value="file">{t('market.import.file')}</SelectItem>
          <SelectItem value="directory">{t('market.import.directory')}</SelectItem>
        </SelectGroup></SelectContent>
      </Select>
    </Field>
    {value.kind === 'file' ? <Field key="file">
      <FieldLabel htmlFor="market-file">{t('market.import.file')}</FieldLabel>
      <Input id="market-file" type="file" accept=".zip,.png,.json" disabled={busy} onChange={event => onChange({ kind: 'file', file: event.target.files?.[0] })} />
    </Field> : value.kind === 'directory' ? <Field key="directory">
      <FieldLabel htmlFor="package-directory">{t('platform.directory')}</FieldLabel>
      <Input id="package-directory" value={value.directory} disabled={busy} onChange={event => onChange({ ...value, directory: event.target.value })} />
      <FieldDescription>{t('market.import.directoryHelp')}</FieldDescription>
    </Field> : <>
      <Field>
        <FieldLabel htmlFor="market-url">{t(value.kind === 'github' ? 'platform.github.repository' : 'market.import.url')}</FieldLabel>
        <Input id="market-url" type="url" value={value.url} disabled={busy} placeholder={value.kind === 'github' ? 'https://github.com/owner/repository' : 'https://example.com/package.zip'} onChange={event => onChange({ ...value, url: event.target.value })} />
      </Field>
      {value.kind === 'github' && <details>
        <summary className="cursor-pointer text-sm text-muted-foreground">{t('market.import.advanced')}</summary>
        <FieldGroup className="mt-3">
          <Field>
            <FieldLabel htmlFor="market-ref">{t('market.import.ref')}</FieldLabel>
            <Input id="market-ref" value={value.ref ?? ''} disabled={busy} placeholder={t('platform.github.defaultBranch')} onChange={event => onChange({ ...value, ref: event.target.value })} />
          </Field>
          <Field>
            <FieldLabel htmlFor="market-path">{t('market.import.path')}</FieldLabel>
            <Input id="market-path" value={value.path ?? ''} disabled={busy} placeholder={t('platform.github.root')} onChange={event => onChange({ ...value, path: event.target.value })} />
          </Field>
          <FieldDescription>{t('platform.github.buildHelp')}</FieldDescription>
          <Button variant="outline" disabled={busy || !value.url.trim()} onClick={() => void run(async () => {
            await importExtensionSource({ url: value.url.trim(), ref: value.ref?.trim() || '', path: value.path?.trim() || '' })
            onImportedSource()
          })}>{t('platform.github.importSource')}</Button>
        </FieldGroup>
      </details>}
    </>}
  </FieldGroup>
}
