import { randomUUID } from '@/lib/uuid'
import { createContext, lazy, Suspense, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { Minimize2, PanelsTopLeft, Puzzle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenuItem } from '@/components/ui/dropdown-menu'
import { toast } from 'sonner'
import { useRemoteAccessLocal } from '@/features/remote-access/api'
import { GamePlayer } from './GamePlayer'
import { localized, management, platformError, releasePluginConsumer, type ContributionContext, type PluginAction, type ProjectPlugin, type RuntimeSnapshot, type LocalizedText } from './api'

const PluginLauncher = lazy(() => import('./PluginLauncher').then(module => ({ default: module.PluginLauncher })))
const PluginCommandDialog = lazy(() => import('./PluginCommandDialog').then(module => ({ default: module.PluginCommandDialog })))

export interface PluginLauncherTarget { projectId?: string; context?: ContributionContext; pluginId?: string }
export interface PluginActionSelection {
  projectId: string
  projectName: string
  context: ContributionContext
  plugin: ProjectPlugin
  action: PluginAction
}
interface OpenPanel { key: string; title: LocalizedText; projectName: string; runtime: RuntimeSnapshot }
const PluginWorkspaceContext = createContext<((target: PluginLauncherTarget) => void) | null>(null)


/** One ephemeral workspace keeps panels bound to their original Project across
 * navigation. It does not persist credentials or invent another product mode. */
export function PluginWorkspaceProvider({ children }: { children: ReactNode }) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  // Panels and commands need a loopback runtime that a LAN browser cannot reach.
  const hostLocal = useRemoteAccessLocal()
  const [launcher, setLauncher] = useState<PluginLauncherTarget | null>(null)
  const [command, setCommand] = useState<PluginActionSelection | null>(null)
  const [panels, setPanels] = useState<OpenPanel[]>([])
  const [active, setActive] = useState('')
  const [visible, setVisible] = useState(false)
  const panelsRef = useRef(panels)
  panelsRef.current = panels
  const opening = useRef(new Set<string>())
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    const unload = () => {
      for (const panel of panelsRef.current) void releasePluginConsumer(panel.runtime).catch(error => console.error('[plugins] release unloaded panel failed', error))
    }
    window.addEventListener('pagehide', unload)
    return () => {
      window.removeEventListener('pagehide', unload)
      mounted.current = false
      for (const panel of panelsRef.current) void releasePluginConsumer(panel.runtime).catch(error => console.error('[plugins] release panel on shutdown failed', error))
    }
  }, [])
  const openLauncher = useCallback((target: PluginLauncherTarget) => setLauncher(target), [])
  const openAction = async (selection: PluginActionSelection) => {
    if (!hostLocal) { toast.error(t('platform.errors.LOCAL_HOST_ONLY')); return }
    if (selection.action.target.kind === 'tool') { setCommand(selection); setLauncher(null); return }
    const key = `${selection.projectId}/${selection.plugin.id}/${selection.action.target.id}`
    if (panelsRef.current.some(panel => panel.key === key)) { setActive(key); setVisible(true); setLauncher(null); return }
    if (opening.current.has(key)) return
    opening.current.add(key)
    try {
      const runtime = await management<RuntimeSnapshot>('/plugin-actions/open', 'POST', {
        projectId: selection.projectId, pluginId: selection.plugin.id, releaseId: selection.plugin.releaseId, context: selection.context,
        kind: selection.action.kind, actionId: selection.action.id, consumerId: randomUUID(),
        locale: i18n.language, theme: resolvedTheme,
      })
      if (!mounted.current) { await releasePluginConsumer(runtime); return }
      setPanels(previous => [...previous, { key, runtime, title: { "zh-CN": `${selection.plugin.name["zh-CN"]} · ${selection.action.title["zh-CN"]}`, "en-US": `${selection.plugin.name["en-US"]} · ${selection.action.title["en-US"]}` }, projectName: selection.projectName }])
      setActive(key); setVisible(true); setLauncher(null)
      console.info('[plugins] panel opened', { plugin: selection.plugin.id, project: selection.projectId, panel: selection.action.target.id })
    } catch (error) {
      console.error('[plugins] open panel failed', { plugin: selection.plugin.id, project: selection.projectId, error })
      toast.error(platformError(error))
    } finally { opening.current.delete(key) }
  }
  const forgetPanel = (key: string) => {
    const remaining = panelsRef.current.filter(panel => panel.key !== key)
    setPanels(remaining)
    if (active === key) setActive(remaining.at(-1)?.key ?? '')
    if (!remaining.length) setVisible(false)
  }
  return <PluginWorkspaceContext.Provider value={openLauncher}>
    {children}
    {launcher && <Suspense fallback={null}><PluginLauncher target={launcher} onClose={() => setLauncher(null)} onSelect={openAction} /></Suspense>}
    {command && <Suspense fallback={null}><PluginCommandDialog selection={command} onClose={() => setCommand(null)} /></Suspense>}
    {panels.length > 0 && <>
      {!visible && <Button className="fixed right-3 bottom-3 z-40 max-w-[calc(100vw-1.5rem)] shadow-md" variant="outline" onClick={() => setVisible(true)}><PanelsTopLeft />{t('platform.plugins.openPanels', { count: panels.length })}</Button>}
      <section role="region" aria-label={t('platform.plugins.workspace')} className={visible ? 'fixed inset-2 z-40 flex min-h-0 min-w-0 flex-col overflow-hidden rounded-lg border bg-background shadow-xl sm:inset-6' : 'hidden'}>
        <header className="flex min-w-0 items-center gap-2 border-b px-2 py-1">
          <div role="tablist" aria-label={t('platform.plugins.panels')} className="flex min-w-0 flex-1 gap-1 overflow-x-auto">
            {panels.map(panel => <Button key={panel.key} role="tab" aria-selected={active === panel.key} variant={active === panel.key ? 'secondary' : 'ghost'} className="max-w-[min(70vw,24rem)] shrink-0" title={`${localized(panel.title, i18n.language)} · ${panel.projectName}`} onClick={() => setActive(panel.key)}><span className="truncate">{localized(panel.title, i18n.language)}</span></Button>)}
          </div>
          <Button variant="ghost" size="icon" aria-label={t('platform.plugins.hide')} onClick={() => setVisible(false)}><Minimize2 /></Button>
        </header>
        <p className="truncate border-b px-3 py-1 text-xs text-muted-foreground" title={panels.find(panel => panel.key === active)?.projectName}>{t('platform.project')}: {panels.find(panel => panel.key === active)?.projectName}</p>
        {panels.map(panel => <div key={panel.key} role="tabpanel" aria-label={`${localized(panel.title, i18n.language)} · ${panel.projectName}`} className={active === panel.key ? 'flex min-h-0 flex-1 flex-col' : 'hidden'}>
          <GamePlayer runtime={panel.runtime} title={`${localized(panel.title, i18n.language)} · ${panel.projectName}`} visible={visible && active === panel.key} variant="plugin" onStop={() => releasePluginConsumer(panel.runtime)} onExit={() => forgetPanel(panel.key)} />
        </div>)}
      </section>
    </>}
  </PluginWorkspaceContext.Provider>
}

/** Explicit product context controls discovery; credentials always bind Project. */
export function PluginActionsButton({ projectId, context, pluginId, label }: PluginLauncherTarget & { label?: string }) {
  const open = useContext(PluginWorkspaceContext)
  const { t } = useTranslation()
  return <Button variant="ghost" size="sm" onClick={() => open?.({ projectId, context, pluginId })}><Puzzle />{label ?? t('platform.plugins.actions')}</Button>
}

export function PluginActionsMenuItem({ projectId, context }: PluginLauncherTarget) {
  const open = useContext(PluginWorkspaceContext)
  const { t } = useTranslation()
  return <DropdownMenuItem onSelect={() => open?.({ projectId, context })}><Puzzle />{t('platform.plugins.actions')}</DropdownMenuItem>
}
