import type { ComponentProps, ReactNode } from 'react'
import { List, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobileWorkspaceHeader } from '@/components/layout/mobile-workspace-header'
import { Button } from '@/components/ui/button'
import { useIsMobile } from '@/hooks/useIsMobile'

type SettingsPageFrameProps = Omit<ComponentProps<typeof FeaturePageShell>, 'icon' | 'leadingContent' | 'headerContent' | 'topbarClassName'> & {
  visible: boolean
  navigation?: ReactNode
}

/** Settings owns its title, save status, and category navigation in one responsive frame.
 * Cached settings must relinquish that slot when another destination is visible. */
export function SettingsPageFrame({ visible, title, actions, navigation, children, ...props }: SettingsPageFrameProps) {
  const isMobile = useIsMobile()
  const { t } = useTranslation()
  return (
    <FeaturePageShell {...props} icon={Settings} title={title} actions={isMobile ? undefined : actions} topbarClassName={isMobile ? 'hidden' : undefined}>
      <AdaptiveSurface
        left={navigation ? {
          id: 'settings-nav',
          title: t('settings.categories'),
          // The desktop sidebar stays on the left; the mobile drawer follows its top-right trigger.
          side: 'right',
          icon: <List className="h-4 w-4" />,
          content: navigation,
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
          mobileClassName: 'w-[min(86vw,340px)]',
        } : undefined}
        className="flex-1 text-xs"
        mainClassName="min-h-0 min-w-0"
        leftResize={{
          layoutKey: 'nova-settings-navigation-layout',
          label: t('layout.resize.sidebar'),
          defaultSize: '224px',
          minSize: '200px',
          maxSize: '36%',
        }}
      >
        {({ openLeft, openPaneId }) => (
          <>
            {isMobile && visible && (
              <MobileWorkspaceHeader route="full">
                <h2 className="nova-mobile-page-title min-w-0 flex-1 truncate text-center">{title}</h2>
                {actions}
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('settings.categories')}
                  aria-haspopup="dialog"
                  aria-expanded={openPaneId === 'settings-nav'}
                  disabled={!navigation}
                  onClick={openLeft}
                >
                  <List />
                </Button>
              </MobileWorkspaceHeader>
            )}
            {children}
          </>
        )}
      </AdaptiveSurface>
    </FeaturePageShell>
  )
}
