import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { getBooks, type BookRecord } from '@/lib/api'
import { exchange, type Installation, type Preview } from './api'
import type { ImportDialogProps } from './ImportDialog'

// An update always addresses an explicit installation, including its Project.
// Choosing a catalog entry alone never transfers resource ownership.
export function MarketInstallationUpdates({ installations, projectID, onImport, onChanged, blockedReason }: {
  projectID?: string
  installations: Installation[]
  onImport: (props: Omit<ImportDialogProps, 'onClose' | 'onInstalled'>) => void
  onChanged: () => Promise<void>
  blockedReason?: string
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState(installations.find(item => item.project_id === projectID)?.installation_id || installations[0]?.installation_id || '')
  const [books, setBooks] = useState<BookRecord[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!installations.length) return
    let alive = true
    void getBooks().then(value => { if (alive) setBooks(value) }).catch(error => { console.error('[market] update destinations unavailable', error) })
    return () => { alive = false }
  }, [installations.length > 0])
  const installation = installations.find(item => item.installation_id === selected) || installations.find(item => item.project_id === projectID) || installations[0]
  if (!installations.length) return null
  return <div className="flex min-w-0 flex-col gap-3">
    <Field>
      <FieldLabel htmlFor="market-update-target">{t('market.update.target')}</FieldLabel>
      <Select value={installation?.installation_id} onValueChange={setSelected} disabled={busy}>
        <SelectTrigger id="market-update-target" className="w-full"><SelectValue /></SelectTrigger>
        <SelectContent><SelectGroup>{installations.map(item => <SelectItem key={item.installation_id} value={item.installation_id}>
          {item.project_id ? books.find(book => book.project_id === item.project_id)?.name || item.project_id : t('market.import.global')} · {item.package.name}
        </SelectItem>)}</SelectGroup></SelectContent>
      </Select>
    </Field>
    <Button disabled={busy || !installation || !!blockedReason} onClick={() => {
      if (!installation) return
      setBusy(true); setError('')
      void exchange<Preview>(`/installations/${installation.installation_id}/check`, {}).then(async preview => {
        await onChanged()
        onImport({ preview, installation })
      }).catch(error => {
        console.error('[market] check installed package failed', error)
        setError(error instanceof Error ? error.message : t('market.errors.operationFailed'))
      }).finally(() => setBusy(false))
    }}>{t(busy ? 'market.working' : 'market.update.installed')}</Button>
    {error && <p role="alert" className="text-sm text-destructive break-words">{error}</p>}
  </div>
}
