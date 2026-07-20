import type { RefObject } from 'react'
import { ChevronDown, ChevronUp, Regex, Replace, Search, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { TooltipIconButton } from '@/components/common/tooltip-icon-button'

interface EditorSearchBarProps {
  inputRef: RefObject<HTMLInputElement | null>
  query: string
  matchIndex: number
  matchCount: number
  useRegex: boolean
  replaceOpen: boolean
  replaceText: string
  onQueryChange: (query: string) => void
  onNavigate: (direction: 1 | -1) => void
  onClose: () => void
  onToggleRegex: () => void
  onToggleReplace: () => void
  onReplaceChange: (text: string) => void
  onReplace: () => void
  onReplaceAll: () => void
}

export function EditorSearchBar({
  inputRef,
  query,
  matchIndex,
  matchCount,
  useRegex,
  replaceOpen,
  replaceText,
  onQueryChange,
  onNavigate,
  onClose,
  onToggleRegex,
  onToggleReplace,
  onReplaceChange,
  onReplace,
  onReplaceAll,
}: EditorSearchBarProps) {
  const { t } = useTranslation()

  return (
    <div className="sticky top-0 z-20 ml-auto mb-3 flex w-[360px] flex-col gap-1 rounded-lg border border-[var(--nova-border)] bg-[var(--nova-menu-bg)] p-1 shadow-xl backdrop-blur">
      <div className="flex items-center gap-1">
        <Search className="ml-2 h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <input
          ref={inputRef}
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              onNavigate(event.shiftKey ? -1 : 1)
            }
            if (event.key === 'Escape') {
              event.preventDefault()
              onClose()
            }
          }}
          placeholder={t('editor.searchPlaceholder')}
          className="min-w-0 flex-1 bg-transparent px-1 py-1 text-xs text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)]"
        />
        <span className="w-14 text-center text-[11px] text-[var(--nova-text-muted)]">
          {matchCount > 0 ? `${matchIndex + 1}/${matchCount}` : '0/0'}
        </span>
        <TooltipIconButton
          label={t('editor.toggleReplace')}
          size="icon-xs"
          className={replaceOpen ? 'text-[var(--nova-accent)] hover:bg-[var(--nova-hover)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}
          onClick={onToggleReplace}
        >
          <Replace className="h-3.5 w-3.5" />
        </TooltipIconButton>
        <TooltipIconButton
          label={t('editor.toggleRegex')}
          size="icon-xs"
          className={useRegex ? 'text-[var(--nova-accent)] hover:bg-[var(--nova-hover)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}
          onClick={onToggleRegex}
        >
          <Regex className="h-3.5 w-3.5" />
        </TooltipIconButton>
        <TooltipIconButton
          label={t('editor.searchPrev')}
          size="icon-xs"
          className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={() => onNavigate(-1)}
          disabled={matchCount === 0}
        >
          <ChevronUp className="h-3.5 w-3.5" />
        </TooltipIconButton>
        <TooltipIconButton
          label={t('editor.searchNext')}
          size="icon-xs"
          className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={() => onNavigate(1)}
          disabled={matchCount === 0}
        >
          <ChevronDown className="h-3.5 w-3.5" />
        </TooltipIconButton>
        <TooltipIconButton
          label={t('editor.closeSearch')}
          size="icon-xs"
          className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={onClose}
        >
          <X className="h-3.5 w-3.5" />
        </TooltipIconButton>
      </div>
      {replaceOpen && (
        <div className="flex items-center gap-1 border-t border-[var(--nova-border)] pt-1">
          <Replace className="ml-2 h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
          <input
            value={replaceText}
            onChange={(event) => onReplaceChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                onReplace()
              }
            }}
            placeholder={t('editor.replacePlaceholder')}
            className="min-w-0 flex-1 bg-transparent px-1 py-1 text-xs text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)]"
          />
          <button
            type="button"
            onClick={onReplace}
            disabled={matchCount === 0}
            className="rounded px-2 py-1 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-40"
          >
            {t('editor.replace')}
          </button>
          <button
            type="button"
            onClick={onReplaceAll}
            disabled={matchCount === 0}
            className="rounded px-2 py-1 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-40"
          >
            {t('editor.replaceAll')}
          </button>
        </div>
      )}
    </div>
  )
}
