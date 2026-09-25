import {
  BookOpen,
  Check,
  Gamepad2,
  GitBranch,
  Package,
  Plug,
  SlidersHorizontal,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { localized, type MarketEntry } from './api'

export function MarketEntryCard({
  entry,
  view,
  acquired,
  onOpen,
}: {
  entry: MarketEntry
  view: string
  acquired: boolean
  onOpen: () => void
}) {
  const { t, i18n } = useTranslation()
  const Icon = entry.kinds.includes('extension.game')
    ? Gamepad2
    : entry.kinds.includes('extension.plugin')
      ? Plug
      : entry.kinds.includes('skill')
        ? Sparkles
        : entry.kinds.some((kind) => kind.startsWith('preset.'))
          ? SlidersHorizontal
          : entry.kinds.includes('lore.item')
            ? BookOpen
            : Package
  const title = localized(entry.name, i18n.language)
  const kindLabel = t(`market.kinds.${entry.kinds[0]}`)
  return (
    <Card
      size="sm"
      data-testid="market-resource-card"
      className={cn(
        'relative min-w-0 transition-shadow hover:ring-foreground/25 focus-within:ring-2 focus-within:ring-ring',
        view === 'list' && 'sm:flex-row sm:items-center sm:gap-2 sm:py-3',
      )}
    >
      <CardHeader
        className={cn('min-w-0 gap-2', view === 'list' && 'sm:flex-1')}
      >
        <CardTitle className="flex min-w-0 items-center gap-2">
          <Icon className="size-4 shrink-0 text-muted-foreground" />
          <button
            className="min-w-0 flex-1 truncate text-left after:absolute after:inset-0 focus:outline-none"
            title={title}
            onClick={onOpen}
          >
            {title}
          </button>
          {acquired && (
            <Check
              className="size-3.5 shrink-0 text-emerald-600"
              aria-label={t('market.acquired')}
            />
          )}
        </CardTitle>
        <CardDescription
          className={cn(
            'line-clamp-2 h-10 break-words',
            view === 'list' && 'sm:line-clamp-1 sm:h-5',
          )}
        >
          {localized(entry.description, i18n.language)}
        </CardDescription>
      </CardHeader>
      <CardContent
        className={cn(
          'flex min-w-0 flex-wrap items-center gap-1.5',
          view === 'list' && 'sm:max-w-56',
        )}
      >
        <Badge
          variant="secondary"
          title={entry.kinds
            .map((kind) => t(`market.kinds.${kind}`))
            .join(' · ')}
        >
          {kindLabel}
          {entry.kinds.length > 1 && (
            <span className="text-muted-foreground">
              +{entry.kinds.length - 1}
            </span>
          )}
        </Badge>
        {entry.tags.slice(0, 2).map((tag) => (
          <Badge variant="outline" key={tag}>
            {t(`market.tag.${tag}`, { defaultValue: tag })}
          </Badge>
        ))}
      </CardContent>
      <CardFooter
        className={cn(
          'justify-between gap-2 px-3 py-2 text-xs text-muted-foreground',
          view === 'list' &&
            'sm:w-44 sm:flex-col sm:items-end sm:gap-1 sm:border-0 sm:bg-transparent sm:py-0',
        )}
      >
        <span className="flex min-w-0 items-center gap-1.5">
          <GitBranch className="size-3 shrink-0" />
          <span className="truncate">{entry.author}</span>
        </span>
        <time className="shrink-0 tabular-nums" dateTime={entry.updated_at}>
          {new Date(`${entry.updated_at}T00:00:00`).toLocaleDateString(
            i18n.language,
          )}
        </time>
      </CardFooter>
    </Card>
  )
}
