import { useState } from 'react'
import { useTranslation } from 'react-i18next'

export interface TurnNavigationItem {
  anchorId: string
  user: string
  narrative: string
  pending?: boolean
}

interface TurnNavigatorProps {
  items: TurnNavigationItem[]
  activeAnchorId?: string
  onSelect: (anchorId: string) => void
}

export function TurnNavigator({ items, activeAnchorId = '', onSelect }: TurnNavigatorProps) {
  const { t } = useTranslation()
  const [previewAnchorId, setPreviewAnchorId] = useState('')
  if (items.length === 0) return null

  return (
    <aside className="nova-turn-navigator" aria-label={t('storyStage.turnNavigator.label')}>
      <div className="nova-turn-navigator-track" role="list">
        {items.map((item, index) => {
          const active = item.anchorId === activeAnchorId
          const previewVisible = previewAnchorId === item.anchorId
          const user = item.user.trim() || t('storyStage.turnNavigator.emptyUser')
          const narrative = item.narrative.trim() || (item.pending ? t('storyStage.turnNavigator.generating') : t('storyStage.turnNavigator.emptyAgent'))
          return (
            <div key={item.anchorId} className="nova-turn-nav-slot" role="listitem">
              <button
                type="button"
                className="nova-turn-nav-button"
                aria-current={active ? 'true' : undefined}
                aria-label={t('storyStage.turnNavigator.goto', { index: index + 1 })}
                data-active={active ? 'true' : undefined}
                data-pending={item.pending ? 'true' : undefined}
                onClick={() => onSelect(item.anchorId)}
                onMouseEnter={() => setPreviewAnchorId(item.anchorId)}
                onMouseLeave={() => setPreviewAnchorId((current) => (current === item.anchorId ? '' : current))}
                onFocus={() => setPreviewAnchorId(item.anchorId)}
                onBlur={() => setPreviewAnchorId((current) => (current === item.anchorId ? '' : current))}
              >
                <span className="nova-turn-nav-mark" aria-hidden="true" />
                {previewVisible ? (
                  <span className="nova-turn-nav-preview" aria-hidden="true">
                    <span className="nova-turn-nav-preview-user">{user}</span>
                    <span className="nova-turn-nav-preview-agent">{narrative}</span>
                  </span>
                ) : null}
              </button>
            </div>
          )
        })}
      </div>
    </aside>
  )
}
