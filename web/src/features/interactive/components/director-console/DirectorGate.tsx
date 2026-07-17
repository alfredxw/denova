import { Eye, ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

// 导演 tab 唯一的防剧透门：节拍表、事件编排与执行过程统一由它保护，揭示一次全部可见。
export function DirectorGate({ onReveal }: { onReveal: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-[280px] items-center justify-center">
      <section className="relative w-full overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-5 py-7 text-center">
        <div className="absolute inset-x-0 top-0 h-px bg-[linear-gradient(90deg,transparent,var(--director-brass),transparent)]" />
        <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--director-brass)]">
          <ShieldAlert className="h-5 w-5" />
        </div>
        <h3 className="director-console__display mt-3 text-base font-semibold text-[var(--nova-text)]">{t('directorPanel.directorSpoilerTitle')}</h3>
        <p className="mt-2 text-xs leading-5 text-[var(--nova-text-muted)]">{t('directorPanel.directorSpoilerDescription')}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="mt-4 gap-2 rounded-[9px] border-[var(--director-brass)] bg-[color-mix(in_srgb,var(--director-brass)_10%,var(--nova-surface))] text-[var(--nova-text)] hover:bg-[color-mix(in_srgb,var(--director-brass)_16%,var(--nova-surface))]"
          onClick={onReveal}
        >
          <Eye className="h-3.5 w-3.5" />
          {t('directorPanel.directorReveal')}
        </Button>
      </section>
    </div>
  )
}
