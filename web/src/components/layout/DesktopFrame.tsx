import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { desktopBridge } from '@/lib/desktop'

/** Windows chrome reserves the native caption area and keeps viewport-sized
 * workspaces below it. Other hosts retain their existing window layout. */
export function DesktopFrame({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const [openMenu, setOpenMenu] = useState<string | null>(null)
  const desktop = desktopBridge()
  if (desktop?.platform !== 'win32') return children

  const menus = [
    { id: 'application', label: t('common.desktopOptions') },
    { id: 'edit', label: t('common.edit') },
    { id: 'view', label: t('common.desktopView') },
  ] as const
  return (
    <div className="nova-desktop-frame">
      <header className="nova-desktop-titlebar">
        <img src="/favicon.svg" alt="" className="size-5 shrink-0" draggable={false} />
        <nav aria-label={t('common.desktopMenu')} className="flex min-w-0 items-center gap-0.5">
          {menus.map(menu => (
            <Button
              key={menu.id} variant="ghost" size="sm" aria-haspopup="menu"
              aria-expanded={openMenu === menu.id}
              // Native Edit commands should keep the editor's selection.
              onMouseDown={event => event.preventDefault()}
              onClick={async event => {
                const bounds = event.currentTarget.getBoundingClientRect()
                setOpenMenu(menu.id)
                try { await desktop.openMenu(menu.id, { x: bounds.left, y: bounds.bottom }) }
                catch (error) {
                  console.error('[desktop] Menu failed to open', menu.id, error)
                  toast.error(t('common.desktopMenuFailed'))
                } finally { setOpenMenu(null) }
              }}
              onKeyDown={event => {
                if (event.key === 'ArrowDown') { event.preventDefault(); event.currentTarget.click() }
                if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
                  event.preventDefault()
                  const buttons = Array.from(event.currentTarget.parentElement!.querySelectorAll('button'))
                  buttons[(buttons.indexOf(event.currentTarget) + (event.key === 'ArrowRight' ? 1 : buttons.length - 1)) % buttons.length].focus()
                }
              }}
            >{menu.label}</Button>
          ))}
        </nav>
      </header>
      <div className="nova-desktop-workspace">{children}</div>
    </div>
  )
}
