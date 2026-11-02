import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { desktopBridge } from '@/lib/desktop'
import { toast } from '@/lib/toast'
import { ReadOnly } from './SettingsFields'

/** Flush settings before the desktop shell can stop the backend and relaunch. */
export function DataDirectorySetting({ value, onBeforeChange }: {
  value?: string
  onBeforeChange: () => Promise<unknown>
}) {
  const { t } = useTranslation()
  const desktop = desktopBridge()
  const [changing, setChanging] = useState(false)

  const changeDirectory = async () => {
    if (!desktop || changing) return
    setChanging(true)
    try {
      await onBeforeChange()
      await desktop.changeDataDirectory()
    } catch (error) {
      console.error('[settings] Failed to change the desktop data directory', error)
      toast.error(t('settings.paths.changeFailed'))
    } finally {
      setChanging(false)
    }
  }

  return (
    <ReadOnly
      label={t('settings.paths.novaDir')}
      value={value}
      description={desktop ? t('settings.paths.changeHint') : undefined}
      action={desktop ? (
        <Button type="button" variant="outline" size="sm" disabled={changing} aria-busy={changing}
          onClick={() => void changeDirectory()}>
          {t('settings.paths.change')}
        </Button>
      ) : undefined}
    />
  )
}
