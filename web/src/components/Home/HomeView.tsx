import { useEffect, useState } from 'react'
import type { ComponentProps, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, rectSortingStrategy, sortableKeyboardCoordinates, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { BookOpen, FileText, Folder, GripVertical, Image as ImageIcon, LibraryBig, Loader2, Pencil, Plus, Sparkles, Trash2, Upload, X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { NovelImportDialog } from './NovelImportDialog'
import {
  bookCoverURL,
  createBook,
  generateBookCover,
  getBookInfo,
  removeBook,
  reorderBooks,
  switchWorkspace,
  updateBookInfo,
  type BookMeta,
  type BookRecord,
} from '@/lib/api'
import { getImagePresets } from '@/features/interactive/api'
import type { ImagePreset } from '@/features/interactive/types'
import { fetchSettings } from '@/features/settings/api'

interface HomeViewProps {
  /** 当前工作区路径，用于高亮当前书籍并作为父目录推断默认值 */
  workspace: string
  /** 用户 Nova 数据目录，新建书籍默认创建在该目录下 */
  novaDir: string
  /** Nova 数据目录下实际存在的书籍 */
  books: BookRecord[]
  /** 切换到指定 workspace 后由父组件刷新业务状态 */
  onSwitch: (path: string) => void
  /** 书籍记录有变更时通知父组件刷新列表 */
  onBooksChange: () => void
  /** 打开酒馆角色卡导入弹窗 */
  onOpenCharacterCardImport?: () => void
  /** 关闭全局书籍管理弹窗 */
  onClose?: () => void
}

const inputCls = 'nova-field w-full rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'
const ghostButtonCls = 'nova-nav-item border border-transparent bg-transparent text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const primaryButtonCls = 'border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]'
const iconButtonCls = 'nova-nav-item text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

/** 书籍管理视图：集中展示、创建、打开和编辑 Nova 数据目录中的书籍。 */
export function HomeView({ workspace, novaDir, books, onSwitch, onBooksChange, onOpenCharacterCardImport, onClose }: HomeViewProps) {
  const { t } = useTranslation()
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [createTitle, setCreateTitle] = useState('')
  const [createAuthor, setCreateAuthor] = useState('')
  const [createDesc, setCreateDesc] = useState('')
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)
  const [showNovelImport, setShowNovelImport] = useState(false)

  const [editingBookPath, setEditingBookPath] = useState<string | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const [editAuthor, setEditAuthor] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [editLoading, setEditLoading] = useState(false)
  const [editSaving, setEditSaving] = useState(false)
  const [imagePresets, setImagePresets] = useState<ImagePreset[]>([])
  const [defaultImagePresetId, setDefaultImagePresetId] = useState('game-cg')
  const [coverPresetId, setCoverPresetId] = useState('game-cg')
  const [coverInstruction, setCoverInstruction] = useState('')
  const [coverGeneratingPath, setCoverGeneratingPath] = useState('')
  const [coverError, setCoverError] = useState('')
  const [coverVersions, setCoverVersions] = useState<Record<string, string>>({})
  const [orderedBooks, setOrderedBooks] = useState<BookRecord[]>(books)
  const [deleteTarget, setDeleteTarget] = useState<BookRecord | null>(null)
  const [deleteError, setDeleteError] = useState('')
  const [deleting, setDeleting] = useState(false)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  useEffect(() => {
    setOrderedBooks(books)
  }, [books])

  useEffect(() => {
    let cancelled = false
    Promise.all([getImagePresets(), fetchSettings()])
      .then(([presets, settings]) => {
        if (cancelled) return
        const nextDefault = settings.effective?.ide_image_preset_id || 'game-cg'
        setImagePresets(presets)
        setDefaultImagePresetId(nextDefault)
        setCoverPresetId((current) => current || nextDefault)
      })
      .catch((err) => {
        console.warn('加载封面图像方案失败', err)
        if (!cancelled) {
          setImagePresets([])
          setDefaultImagePresetId('game-cg')
          setCoverPresetId((current) => current || 'game-cg')
        }
      })
    return () => { cancelled = true }
  }, [])

  /** 打开新建书籍表单，新书统一创建在用户 Nova 数据目录下 */
  const openCreateForm = () => {
    setShowCreateForm(true)
    setCreateTitle('')
    setCreateAuthor('')
    setCreateDesc('')
    setCreateError('')
  }

  /** 提交新建书籍 */
  const handleCreate = async () => {
    if (!createTitle.trim()) { setCreateError(t('home.titleRequired')); return }
    if (!novaDir.trim()) { setCreateError(t('home.waitNovaDir')); return }
    setCreating(true)
    setCreateError('')
    try {
      const data = await createBook(
        createTitle.trim(),
        createAuthor.trim() || undefined,
        createDesc.trim() || undefined,
      )
      onSwitch(data.workspace)
      setShowCreateForm(false)
      onBooksChange()
    } catch (e: unknown) {
      setCreateError(e instanceof Error ? e.message : t('home.createError'))
    } finally {
      setCreating(false)
    }
  }

  /** 切换到指定书籍 */
  const handleSwitch = async (path: string) => {
    try {
      const data = await switchWorkspace(path)
      onSwitch(data.workspace || path)
    } catch (e) {
      console.error('切换 workspace 失败', e)
    }
  }

  /** 进入编辑模式，先拉取完整元信息 */
  const startEdit = async (book: BookRecord) => {
    setEditingBookPath(book.path)
    setEditTitle(book.name)
    setEditAuthor(book.author || '')
    setEditDesc('')
    setCoverPresetId(defaultImagePresetId || 'game-cg')
    setCoverInstruction('')
    setCoverError('')
    setEditLoading(true)
    try {
      const meta: BookMeta = await getBookInfo(book.path)
      setEditTitle(meta.title)
      setEditAuthor(meta.author)
      setEditDesc(meta.description)
    } catch {
      // 拉取失败时回退使用列表里的基础信息
    } finally {
      setEditLoading(false)
    }
  }

  /** 保存书籍编辑 */
  const handleSaveEdit = async () => {
    if (!editingBookPath) return
    setEditSaving(true)
    try {
      await updateBookInfo(editingBookPath, editTitle.trim(), editAuthor.trim(), editDesc.trim())
      setEditingBookPath(null)
      onBooksChange()
    } catch (e) {
      console.error('保存书籍信息失败', e)
    } finally {
      setEditSaving(false)
    }
  }

  const handleGenerateCover = async (book: BookRecord) => {
    setCoverGeneratingPath(book.path)
    setCoverError('')
    try {
      const result = await generateBookCover({
        path: book.path,
        imagePresetId: coverPresetId || defaultImagePresetId || 'game-cg',
        instruction: coverInstruction.trim(),
      })
      setCoverVersions((current) => ({
        ...current,
        [book.path]: result.cover_updated_at || String(Date.now()),
      }))
      await Promise.resolve(onBooksChange())
    } catch (e) {
      setCoverError(e instanceof Error ? e.message : t('home.coverGenerateError'))
    } finally {
      setCoverGeneratingPath('')
    }
  }

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = orderedBooks.findIndex((book) => book.path === active.id)
    const newIndex = orderedBooks.findIndex((book) => book.path === over.id)
    if (oldIndex === -1 || newIndex === -1) return
    const nextBooks = arrayMove(orderedBooks, oldIndex, newIndex)
    setOrderedBooks(nextBooks)
    try {
      await reorderBooks(nextBooks.map((book) => book.path))
      await onBooksChange()
    } catch (e) {
      console.error('保存书籍排序失败', e)
      setOrderedBooks(books)
    }
  }

  const openDeleteDialog = (book: BookRecord) => {
    setDeleteTarget(book)
    setDeleteError('')
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    setDeleteError('')
    try {
      const result = await removeBook(deleteTarget.path)
      if (deleteTarget.path === workspace) {
        onSwitch(result.workspace || '')
      } else {
        await onBooksChange()
      }
      setDeleteTarget(null)
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : String(e))
    } finally {
      setDeleting(false)
    }
  }

  const currentBook = orderedBooks.find((book) => book.path === workspace)
  const editingBook = editingBookPath ? orderedBooks.find((book) => book.path === editingBookPath) || null : null
  const validImagePresets = imagePresets.filter((preset) => !preset.invalid)
  const imagePresetOptions = validImagePresets.length > 0
    ? validImagePresets
    : [{ id: defaultImagePresetId || 'game-cg', name: t('home.coverDefaultPreset') } as ImagePreset]
  const coverVersion = (book: BookRecord) => coverVersions[book.path] || book.cover_updated_at || ''

  return (
    <div className="nova-sidebar flex h-full min-w-0 flex-col text-[var(--nova-text)]">
      <div className="nova-topbar flex h-10 shrink-0 items-center gap-2 border-b px-4 text-xs">
        <LibraryBig className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <span className="font-medium text-[var(--nova-text)]">{t('home.title')}</span>
        <span className="text-[11px] text-[var(--nova-text-faint)]">{t('home.bookCount', { count: books.length })}</span>
        {onClose && (
          <button
            type="button"
            onClick={onClose}
            className={`${iconButtonCls} ml-auto rounded p-1`}
            aria-label={t('home.close')}
            title={t('home.close')}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="mx-auto flex w-full min-w-0 max-w-full flex-col gap-5 overflow-x-hidden px-4 py-5 sm:max-w-4xl sm:px-6 sm:py-6">
          {/* 当前书籍 */}
          <section className="min-w-0 border-b border-[var(--nova-border)] pb-5">
            <div className="mb-2 flex items-center gap-2 text-[11px] font-medium uppercase text-[var(--nova-text-faint)]">
              <BookOpen className="h-3.5 w-3.5" />
              {t('home.currentBook')}
            </div>
            <div className="flex min-w-0 flex-col gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.035)] sm:flex-row sm:items-center sm:justify-between">
              <div className="flex min-w-0 items-center gap-3">
                {currentBook && (
                  <BookCoverThumbnail
                    book={currentBook}
                    version={coverVersion(currentBook)}
                    className="h-16 w-12 shrink-0"
                    iconClassName="h-4 w-4"
                  />
                )}
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold text-[var(--nova-text)]">
                    {currentBook?.name || (workspace ? workspace.split('/').filter(Boolean).pop() : t('home.currentWorkspaceUnset'))}
                  </div>
                  <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">{workspace || t('home.startHint')}</div>
                </div>
              </div>
              {currentBook && (
                <div className="flex shrink-0 items-center gap-1.5 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
                  <BookOpen className="h-3 w-3" />
                  {t('common.current')}
                </div>
              )}
            </div>
          </section>

          {/* 书籍列表 */}
          <section className="min-w-0">
            <div className="mb-3 flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-center gap-2 text-[11px] font-medium uppercase text-[var(--nova-text-faint)]">
                <Folder className="h-3.5 w-3.5" />
                {t('home.bookshelf')}
              </div>
              <div className="flex w-full min-w-0 flex-wrap items-center justify-start gap-2 sm:w-auto sm:shrink-0 sm:justify-end">
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  className={`${ghostButtonCls} max-w-full`}
                  onClick={() => setShowNovelImport(true)}
                >
                  <FileText className="h-3.5 w-3.5" />
                  {t('home.importNovel')}
                </Button>
                {onOpenCharacterCardImport && (
                  <Button
                    type="button"
                    size="xs"
                    variant="ghost"
                    className={`${ghostButtonCls} max-w-full`}
                    onClick={onOpenCharacterCardImport}
                  >
                    <Upload className="h-3.5 w-3.5" />
                    {t('home.importCard')}
                  </Button>
                )}
                {!showCreateForm && books.length > 0 && (
                  <Button
                    type="button"
                    size="xs"
                    variant="ghost"
                    className={`${ghostButtonCls} max-w-full`}
                    onClick={openCreateForm}
                  >
                    <Plus className="h-3.5 w-3.5" />
                    {t('home.createBook')}
                  </Button>
                )}
              </div>
            </div>

            {showCreateForm && (
              <div className="mb-4 space-y-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.035)]">
                <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
                  <Plus className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
                  {t('home.createBook')}
                </div>
                <Input
                  type="text"
                  value={createTitle}
                  onChange={(e) => setCreateTitle(e.target.value)}
                  placeholder={t('home.bookTitlePlaceholder')}
                  className={inputCls}
                  autoFocus
                />
                <Input
                  type="text"
                  value={createAuthor}
                  onChange={(e) => setCreateAuthor(e.target.value)}
                  placeholder={t('home.authorPlaceholder')}
                  className={inputCls}
                />
                <div className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5 text-xs text-[var(--nova-text-faint)]">
                  <Folder className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                  <span className="shrink-0">{t('home.createIn')}</span>
                  <span className="truncate text-[var(--nova-text-muted)]">{novaDir || t('home.novaDirLoading')}</span>
                </div>
                <Textarea
                  autoResize
                  value={createDesc}
                  onChange={(e) => setCreateDesc(e.target.value)}
                  placeholder={t('home.descriptionPlaceholder')}
                  rows={1}
                  className={inputCls + ' min-h-0 resize-none'}
                />
                {createError && <div className="text-xs text-[var(--nova-danger)]">{createError}</div>}
                <div className="flex items-center justify-end gap-2">
                  <Button type="button" size="xs" variant="ghost" className={ghostButtonCls} onClick={() => setShowCreateForm(false)}>{t('common.cancel')}</Button>
                  <Button type="button" size="xs" className={primaryButtonCls} disabled={creating || !novaDir.trim()} onClick={handleCreate}>
                    {creating ? t('common.creating') : t('common.create')}
                  </Button>
                </div>
              </div>
            )}

            {orderedBooks.length === 0 ? (
              <div className="flex flex-col items-center gap-3 rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-8 text-center text-xs text-[var(--nova-text-faint)]">
                <div className="text-sm font-medium text-[var(--nova-text-muted)]">{t('home.empty')}</div>
                <div className="max-w-md leading-5">{t('home.emptyDescription')}</div>
                {!showCreateForm && (
                  <Button
                    type="button"
                    size="xs"
                    className={primaryButtonCls}
                    onClick={openCreateForm}
                  >
                    <Plus className="h-3.5 w-3.5" />
                    {t('home.createBook')}
                  </Button>
                )}
              </div>
            ) : (
              <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
                <SortableContext items={orderedBooks.map((book) => book.path)} strategy={rectSortingStrategy}>
                  <div className="grid w-full max-w-[calc(100vw-2rem)] min-w-0 grid-cols-[repeat(auto-fit,minmax(104px,1fr))] gap-2.5 min-[520px]:grid-cols-[repeat(auto-fill,minmax(132px,1fr))] sm:max-w-none sm:grid-cols-[repeat(auto-fill,minmax(168px,1fr))] sm:gap-3">
                    {orderedBooks.map((book) => {
                      const isCurrent = book.path === workspace

                      return (
                        <SortableBookCard
                          key={book.path}
                          book={book}
                        >
                          {(dragHandleProps) => (
                            <div
                              className={`group relative overflow-hidden rounded-[var(--nova-radius)] border text-xs transition-colors sm:min-h-[232px] ${
                                isCurrent
                                  ? 'border-[var(--nova-accent)] bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.04)]'
                                  : 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]'
                              }`}
                            >
                              {isCurrent && (
                                <div className="absolute left-0 top-0 bottom-0 w-[4px] bg-[var(--nova-accent)]" />
                              )}
                              <div className="pointer-events-none absolute inset-x-0 bottom-0 hidden h-3 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] sm:block" />
                              <button
                                type="button"
                                className="flex h-full w-full min-w-0 flex-col items-center px-2 py-2 text-center sm:min-h-[232px] sm:items-stretch sm:px-3 sm:py-3 sm:text-left"
                                onClick={() => handleSwitch(book.path)}
                              >
                                <BookCoverThumbnail
                                  book={book}
                                  version={coverVersion(book)}
                                  className="mb-2 aspect-[3/4] w-full shrink-0 sm:mb-3 sm:w-full sm:shrink"
                                  iconClassName={`h-5 w-5 ${isCurrent ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`}
                                />
                                <div className="flex min-w-0 flex-1 flex-col self-stretch">
                                  <div className="line-clamp-2 text-xs font-semibold leading-4 text-[var(--nova-text)] sm:text-sm sm:leading-5">{book.name || t('home.unnamedBook')}</div>
                                  {book.author && <div className="hidden truncate text-[11px] text-[var(--nova-text-muted)] sm:mt-2 sm:block">{book.author}</div>}
                                  <div className="mt-auto hidden truncate pt-4 text-[10px] text-[var(--nova-text-faint)] sm:block">{book.path}</div>
                                </div>
                              </button>
                              <div className="absolute right-2 top-2 z-10 flex shrink-0 items-center gap-0.5">
                                <TooltipIconButton
                                  label={t('home.dragToSort')}
                                  className={`${iconButtonCls} cursor-grab bg-[var(--nova-surface)] opacity-100 sm:pointer-events-none sm:opacity-0 sm:group-hover:pointer-events-auto sm:group-hover:opacity-100`}
                                  {...dragHandleProps}
                                >
                                  <GripVertical className="h-3.5 w-3.5" />
                                </TooltipIconButton>
                                <TooltipIconButton
                                  label={t('home.editInfo')}
                                  className={`${iconButtonCls} bg-[var(--nova-surface)] opacity-100 sm:pointer-events-none sm:opacity-0 sm:group-hover:pointer-events-auto sm:group-hover:opacity-100`}
                                  onClick={() => startEdit(book)}
                                >
                                  <Pencil className="h-3.5 w-3.5" />
                                </TooltipIconButton>
                                <TooltipIconButton
                                  label={t('home.deleteBook')}
                                  className={`${iconButtonCls} bg-[var(--nova-surface)] text-[var(--nova-danger)] opacity-100 hover:text-[var(--nova-danger)] sm:pointer-events-none sm:opacity-0 sm:group-hover:pointer-events-auto sm:group-hover:opacity-100`}
                                  onClick={() => openDeleteDialog(book)}
                                >
                                  <Trash2 className="h-3.5 w-3.5" />
                                </TooltipIconButton>
                                {isCurrent && (
                                  <span className="hidden rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)] sm:inline">
                                    {t('common.current')}
                                  </span>
                                )}
                              </div>
                            </div>
                          )}
                        </SortableBookCard>
                      )
                    })}
                  </div>
                </SortableContext>
              </DndContext>
            )}
          </section>

        </div>
      </ScrollArea>
      <NovelImportDialog
        open={showNovelImport}
        novaDir={novaDir}
        onOpenChange={setShowNovelImport}
        onImported={(result) => {
          onSwitch(result.workspace)
          onBooksChange()
          onClose?.()
        }}
      />
      <Dialog open={Boolean(editingBook)} onOpenChange={(open) => {
        if (!open && !editSaving) setEditingBookPath(null)
      }}>
        <DialogContent
          showCloseButton={false}
          className="nova-panel flex max-h-[min(760px,calc(100vh-2rem))] w-[min(720px,calc(100vw-2rem))] max-w-[min(720px,calc(100vw-2rem))] grid-rows-none flex-col gap-0 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
        >
          <DialogHeader className="border-b border-[var(--nova-border)] px-4 py-3">
            <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{t('home.editInfo')}</DialogTitle>
            <DialogDescription className="truncate text-xs text-[var(--nova-text-faint)]">
              {editingBook?.path || ''}
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
            {editLoading || !editingBook ? (
              <div className="py-10 text-center text-xs text-[var(--nova-text-faint)]">{t('common.loading')}</div>
            ) : (
              <div className="grid min-w-0 gap-4 md:grid-cols-[minmax(0,1fr)_180px]">
                <div className="min-w-0 space-y-3">
                  <Input
                    type="text"
                    value={editTitle}
                    onChange={(e) => setEditTitle(e.target.value)}
                    placeholder={t('home.bookTitlePlaceholder')}
                    className={inputCls}
                    autoFocus
                  />
                  <Input
                    type="text"
                    value={editAuthor}
                    onChange={(e) => setEditAuthor(e.target.value)}
                    placeholder={t('home.authorPlaceholder')}
                    className={inputCls}
                  />
                  <Textarea
                    autoResize
                    value={editDesc}
                    onChange={(e) => setEditDesc(e.target.value)}
                    placeholder={t('home.descriptionPlaceholder')}
                    rows={7}
                    className={inputCls + ' min-h-36 resize-none'}
                  />
                </div>
                <div className="min-w-0 space-y-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
                  <BookCoverThumbnail
                    book={editingBook}
                    version={coverVersion(editingBook)}
                    className="mx-auto aspect-[3/4] w-28 md:w-full"
                    iconClassName="h-5 w-5"
                  />
                  <div className="flex items-center gap-1.5 text-[11px] font-medium text-[var(--nova-text-muted)]">
                    <ImageIcon className="h-3.5 w-3.5" />
                    {t('home.cover')}
                  </div>
                  <select
                    aria-label={t('home.coverPreset')}
                    value={coverPresetId || defaultImagePresetId || 'game-cg'}
                    onChange={(event) => setCoverPresetId(event.target.value)}
                    className={inputCls + ' h-8 py-1 text-xs'}
                  >
                    {imagePresetOptions.map((preset) => (
                      <option key={preset.id} value={preset.id}>{preset.name || preset.id}</option>
                    ))}
                  </select>
                  <Textarea
                    autoResize
                    value={coverInstruction}
                    onChange={(event) => setCoverInstruction(event.target.value)}
                    placeholder={t('home.coverInstructionPlaceholder')}
                    rows={3}
                    className={inputCls + ' min-h-20 resize-none'}
                  />
                  {coverError && <div className="line-clamp-3 text-[11px] text-[var(--nova-danger)]">{coverError}</div>}
                  <Button
                    type="button"
                    size="xs"
                    className={primaryButtonCls + ' w-full max-w-full justify-center'}
                    disabled={coverGeneratingPath === editingBook.path}
                    onClick={() => void handleGenerateCover(editingBook)}
                  >
                    {coverGeneratingPath === editingBook.path ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Sparkles className="h-3.5 w-3.5" />}
                    {coverGeneratingPath === editingBook.path ? t('home.coverGenerating') : t('home.generateCover')}
                  </Button>
                </div>
              </div>
            )}
          </div>
          <DialogFooter className="!mx-0 !mb-0 rounded-none border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)]/95 !px-4 !py-3">
            <Button
              type="button"
              size="xs"
              variant="ghost"
              className={ghostButtonCls}
              disabled={editSaving}
              onClick={() => setEditingBookPath(null)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              size="xs"
              className={primaryButtonCls}
              disabled={editSaving || editLoading || !editingBook}
              onClick={handleSaveEdit}
            >
              {editSaving ? t('common.saving') : t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AlertDialog open={Boolean(deleteTarget)} onOpenChange={(open) => {
        if (!open && !deleting) setDeleteTarget(null)
      }}>
        <AlertDialogContent className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
          <AlertDialogHeader>
            <AlertDialogTitle>{t('home.deleteBook')}</AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--nova-text-muted)]">
              {t('home.deleteBookDescription', { name: deleteTarget?.name || t('home.unnamedBook') })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2 text-xs text-[var(--nova-text-faint)]">
            {deleteTarget?.path}
          </div>
          {deleteError && <div className="text-xs text-[var(--nova-danger)]">{deleteError}</div>}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]"
              disabled={deleting}
              onClick={(e) => {
                e.preventDefault()
                void handleDelete()
              }}
            >
              {t('home.softDeleteBook')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function BookCoverThumbnail({ book, version, className, iconClassName }: {
  book: BookRecord
  version?: string
  className?: string
  iconClassName?: string
}) {
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setFailed(false)
  }, [book.path, version])

  const showImage = !failed
  return (
    <div
      className={`relative flex min-w-0 items-center justify-center overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] ${className || ''}`}
      title={book.name || book.path}
    >
      {showImage ? (
        <img
          src={bookCoverURL(book.path, version)}
          alt={book.name || book.path}
          className="h-full w-full object-cover"
          onError={() => setFailed(true)}
        />
      ) : (
        <BookOpen className={iconClassName || 'h-4 w-4 text-[var(--nova-text-muted)]'} />
      )}
    </div>
  )
}

function SortableBookCard({ book, disabled, children }: {
  book: BookRecord
  disabled?: boolean
  children: (dragHandleProps: ComponentProps<'button'>) => ReactNode
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: book.path, disabled })
  const dragHandleProps: ComponentProps<'button'> = disabled
    ? {}
    : { ...attributes, ...listeners }

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={isDragging ? 'relative z-10 opacity-80' : undefined}
    >
      {children(dragHandleProps)}
    </div>
  )
}
