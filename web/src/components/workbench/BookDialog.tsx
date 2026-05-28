import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { HomeView } from '@/components/Home/HomeView'
import type { BookRecord } from '@/lib/api'

interface BookDialogProps {
  open: boolean
  workspace: string
  novaDir: string
  books: BookRecord[]
  onOpenChange: (open: boolean) => void
  onSwitch: (path: string) => void
  onBooksChange: () => void | Promise<void>
}

export function BookDialog({
  open,
  workspace,
  novaDir,
  books,
  onOpenChange,
  onSwitch,
  onBooksChange,
}: BookDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="nova-panel left-[2vw] top-[4vh] h-[92vh] max-h-[96vh] min-h-[520px] w-[96vw] max-w-[96vw] min-w-[min(760px,96vw)] translate-x-0 translate-y-0 resize overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)] sm:max-w-[96vw]"
        showCloseButton={false}
        aria-describedby={undefined}
      >
        <DialogTitle className="sr-only">书籍管理</DialogTitle>
        <HomeView
          workspace={workspace}
          novaDir={novaDir}
          books={books}
          onSwitch={onSwitch}
          onBooksChange={onBooksChange}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  )
}
