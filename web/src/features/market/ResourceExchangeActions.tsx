import { useState } from 'react'
import { Download, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { ImportDialog } from './ImportDialog'
import { ExportDialog } from './ExportDialog'
import type { LocalRef } from './api'

/** Domain editors keep their autosave and refresh lifecycle around shared I/O. */
export function ResourceExchangeActions({
  projectID,
  resources,
  beforeOpen,
  onImported,
}: {
  projectID?: string
  resources?: LocalRef[]
  beforeOpen?: () => Promise<boolean>
  onImported: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState<'import' | 'export'>()
  const [busy, setBusy] = useState(false)
  const show = async (kind: 'import' | 'export') => {
    setBusy(true)
    try {
      if (!beforeOpen || (await beforeOpen())) setOpen(kind)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('market.errors.operationFailed'),
      )
    } finally {
      setBusy(false)
    }
  }
  return (
    <>
      <Button
        variant="outline"
        size="icon-sm"
        aria-label={t('market.import.title')}
        title={t('market.import.title')}
        disabled={busy}
        onClick={() => void show('import')}
      >
        <Download />
      </Button>
      <Button
        variant="outline"
        size="icon-sm"
        aria-label={t('market.export.title')}
        title={t('market.export.title')}
        disabled={busy}
        onClick={() => void show('export')}
      >
        <Upload />
      </Button>
      {open === 'import' && (
        <ImportDialog
          projectID={projectID}
          onClose={() => setOpen(undefined)}
          onInstalled={onImported}
        />
      )}
      {open === 'export' && (
        <ExportDialog
          projectID={projectID}
          initialResources={resources}
          onClose={() => setOpen(undefined)}
        />
      )}
    </>
  )
}
