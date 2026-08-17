import { useMemo, useState } from 'react'
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

const MAX_TURN_NAVIGATION_MARKS = 28

interface AggregatedTurnNavigationItem {
  item: TurnNavigationItem
  sourceIndex: number
}

export function TurnNavigator({ items, activeAnchorId = '', onSelect }: TurnNavigatorProps) {
  const { t } = useTranslation()
  const [previewAnchorId, setPreviewAnchorId] = useState('')
  const navigationItems = useMemo(
    () => aggregateTurnNavigationItems(items, activeAnchorId),
    [activeAnchorId, items],
  )
  if (items.length === 0) return null

  return (
    <aside className="nova-turn-navigator" aria-label={t('storyStage.turnNavigator.label')}>
      <div className="nova-turn-navigator-track" role="list">
        {navigationItems.map(({ item, sourceIndex }) => {
          const active = item.anchorId === activeAnchorId
          const previewVisible = previewAnchorId === item.anchorId
          const user = item.user.trim() || t('storyStage.turnNavigator.emptyUser')
          const narrative = item.narrative.trim() || (item.pending ? t('storyStage.turnNavigator.generating') : t('storyStage.turnNavigator.emptyAgent'))
          return (
            <div key={item.anchorId} className="nova-turn-nav-slot" role="listitem" aria-posinset={sourceIndex + 1} aria-setsize={items.length}>
              <button
                type="button"
                className="nova-turn-nav-button"
                aria-current={active ? 'true' : undefined}
                aria-label={t('storyStage.turnNavigator.goto', { index: sourceIndex + 1 })}
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

export function aggregateTurnNavigationItems(
  items: TurnNavigationItem[],
  activeAnchorId = '',
  maxMarks = MAX_TURN_NAVIGATION_MARKS,
): AggregatedTurnNavigationItem[] {
  if (items.length <= maxMarks || maxMarks < 3) {
    return items.map((item, sourceIndex) => ({ item, sourceIndex }))
  }
  const selected = new Set<number>([0, items.length - 1])
  const activeIndex = items.findIndex((item) => item.anchorId === activeAnchorId)
  if (activeIndex >= 0) selected.add(activeIndex)
  items.forEach((item, index) => {
    if (item.pending) selected.add(index)
  })
  for (let slot = 0; slot < maxMarks; slot += 1) {
    selected.add(Math.round((slot * (items.length - 1)) / (maxMarks - 1)))
  }
  return [...selected]
    .sort((left, right) => left - right)
    .map((sourceIndex) => ({ item: items[sourceIndex], sourceIndex }))
}
