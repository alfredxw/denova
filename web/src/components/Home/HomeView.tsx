import { useState } from 'react'
import { BookOpen, Check, Clock3, Folder, LibraryBig, Pencil, Plus, Trash2, X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import {
  createBook,
  getBookInfo,
  removeBook,
  switchWorkspace,
  updateBookInfo,
  type BookMeta,
  type BookRecord,
} from '@/lib/api'

interface HomeViewProps {
  /** 当前工作区路径，用于高亮当前书籍并作为父目录推断默认值 */
  workspace: string
  /** 用户 Nova 数据目录，新建书籍默认创建在该目录下 */
  novaDir: string
  /** 最近书籍列表 */
  books: BookRecord[]
  /** 切换到指定 workspace 后由父组件刷新业务状态 */
  onSwitch: (path: string) => void
  /** 书籍记录有变更时通知父组件刷新列表 */
  onBooksChange: () => void
  /** 关闭全局书籍管理弹窗 */
  onClose?: () => void
}

/** 计算相对时间描述 */
function relativeTime(isoStr: string): string {
  if (!isoStr) return ''
  const diff = Date.now() - new Date(isoStr).getTime()
  if (diff < 0) return '刚刚'
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}天前`
  const months = Math.floor(days / 30)
  return `${months}月前`
}

const inputCls = 'nova-field w-full rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[#3a3a3a] focus:bg-[var(--nova-surface-3)]'
const ghostButtonCls = 'nova-nav-item border border-transparent bg-transparent text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const primaryButtonCls = 'border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]'
const iconButtonCls = 'nova-nav-item text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

/** 书籍管理视图：集中展示、创建、打开和编辑最近书籍。 */
export function HomeView({ workspace, novaDir, books, onSwitch, onBooksChange, onClose }: HomeViewProps) {
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [createTitle, setCreateTitle] = useState('')
  const [createAuthor, setCreateAuthor] = useState('')
  const [createDesc, setCreateDesc] = useState('')
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)

  const [editingBookPath, setEditingBookPath] = useState<string | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const [editAuthor, setEditAuthor] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [editLoading, setEditLoading] = useState(false)
  const [editSaving, setEditSaving] = useState(false)

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
    if (!createTitle.trim()) { setCreateError('书名不能为空'); return }
    if (!novaDir.trim()) { setCreateError('Nova 数据目录未就绪，请稍后重试'); return }
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
      setCreateError(e instanceof Error ? e.message : '创建失败')
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

  /** 移除最近书籍记录（不删除磁盘内容） */
  const handleRemove = async (path: string) => {
    try {
      await removeBook(path)
      onBooksChange()
    } catch (e) {
      console.error('移除书籍记录失败', e)
    }
  }

  /** 进入编辑模式，先拉取完整元信息 */
  const startEdit = async (book: BookRecord) => {
    setEditingBookPath(book.path)
    setEditTitle(book.name)
    setEditAuthor(book.author || '')
    setEditDesc('')
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

  const currentBook = books.find((book) => book.path === workspace)

  return (
    <div className="nova-sidebar flex h-full min-w-0 flex-col text-[var(--nova-text)]">
      <div className="nova-topbar flex h-10 shrink-0 items-center gap-2 border-b px-4 text-xs">
        <LibraryBig className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <span className="font-medium text-[var(--nova-text)]">书籍管理</span>
        <span className="text-[11px] text-[var(--nova-text-faint)]">{books.length} 本最近书籍</span>
        {onClose && (
          <button
            type="button"
            onClick={onClose}
            className={`${iconButtonCls} ml-auto rounded p-1`}
            aria-label="关闭书籍管理"
            title="关闭书籍管理"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      <ScrollArea className="flex-1">
        <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-6">
          {/* 当前书籍 */}
          <section className="border-b border-[var(--nova-border)] pb-5">
            <div className="mb-2 flex items-center gap-2 text-[11px] font-medium uppercase text-[var(--nova-text-faint)]">
              <BookOpen className="h-3.5 w-3.5" />
              当前书籍
            </div>
            <div className="flex min-w-0 flex-col gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.035)] sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <div className="truncate text-sm font-semibold text-[var(--nova-text)]">
                  {currentBook?.name || (workspace ? workspace.split('/').filter(Boolean).pop() : '未设置工作区')}
                </div>
                <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">{workspace || '请新建或选择一本书开始写作'}</div>
              </div>
              {currentBook?.last_opened_at && (
                <div className="flex shrink-0 items-center gap-1.5 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
                  <Clock3 className="h-3 w-3" />
                  {relativeTime(currentBook.last_opened_at)}
                </div>
              )}
            </div>
          </section>

          {/* 书籍列表 */}
          <section>
            <div className="mb-3 flex items-center justify-between">
              <div className="flex items-center gap-2 text-[11px] font-medium uppercase text-[var(--nova-text-faint)]">
                <Folder className="h-3.5 w-3.5" />
                最近书籍
              </div>
              {!showCreateForm && (
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  className={ghostButtonCls}
                  onClick={openCreateForm}
                >
                  <Plus className="h-3.5 w-3.5" />
                  新建书籍
                </Button>
              )}
            </div>

            {showCreateForm && (
              <div className="mb-4 space-y-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.035)]">
                <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
                  <Plus className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
                  新建书籍
                </div>
                <Input
                  type="text"
                  value={createTitle}
                  onChange={(e) => setCreateTitle(e.target.value)}
                  placeholder="书名（必填）"
                  className={inputCls}
                  autoFocus
                />
                <Input
                  type="text"
                  value={createAuthor}
                  onChange={(e) => setCreateAuthor(e.target.value)}
                  placeholder="作者（选填）"
                  className={inputCls}
                />
                <div className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5 text-xs text-[var(--nova-text-faint)]">
                  <Folder className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                  <span className="shrink-0">新书将创建在</span>
                  <span className="truncate text-[var(--nova-text-muted)]">{novaDir || 'Nova 数据目录加载中...'}</span>
                </div>
                <Textarea
                  value={createDesc}
                  onChange={(e) => setCreateDesc(e.target.value)}
                  placeholder="简介（选填）"
                  rows={3}
                  className={inputCls + ' min-h-0 resize-none'}
                />
                {createError && <div className="text-xs text-red-400">{createError}</div>}
                <div className="flex items-center justify-end gap-2">
                  <Button type="button" size="xs" variant="ghost" className={ghostButtonCls} onClick={() => setShowCreateForm(false)}>取消</Button>
                  <Button type="button" size="xs" className={primaryButtonCls} disabled={creating || !novaDir.trim()} onClick={handleCreate}>
                    {creating ? '创建中...' : '创建'}
                  </Button>
                </div>
              </div>
            )}

            {books.length === 0 ? (
              <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-8 text-center text-xs text-[var(--nova-text-faint)]">暂无书籍记录</div>
            ) : (
              <div className="space-y-2">
                {books.map((book) => {
                  const isCurrent = book.path === workspace
                  const isEditing = editingBookPath === book.path

                  if (isEditing) {
                    return (
                      <div key={book.path} className="space-y-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.035)]">
                        {editLoading ? (
                          <div className="py-2 text-center text-xs text-[var(--nova-text-faint)]">加载中...</div>
                        ) : (
                          <>
                            <Input
                              type="text"
                              value={editTitle}
                              onChange={(e) => setEditTitle(e.target.value)}
                              placeholder="书名"
                              className={inputCls}
                              autoFocus
                            />
                            <Input
                              type="text"
                              value={editAuthor}
                              onChange={(e) => setEditAuthor(e.target.value)}
                              placeholder="作者"
                              className={inputCls}
                            />
                            <Textarea
                              value={editDesc}
                              onChange={(e) => setEditDesc(e.target.value)}
                              placeholder="简介"
                              rows={2}
                              className={inputCls + ' min-h-0 resize-none'}
                            />
                            <div className="flex items-center justify-end gap-2">
                              <TooltipIconButton
                                label="取消"
                                className={iconButtonCls}
                                onClick={() => setEditingBookPath(null)}
                              >
                                <X className="h-3.5 w-3.5" />
                              </TooltipIconButton>
                              <TooltipIconButton
                                label="保存"
                                className="nova-nav-item text-[var(--nova-accent-green)] hover:bg-[var(--nova-hover)]"
                                disabled={editSaving}
                                onClick={handleSaveEdit}
                              >
                                <Check className="h-3.5 w-3.5" />
                              </TooltipIconButton>
                            </div>
                          </>
                        )}
                      </div>
                    )
                  }

                  return (
                    <div
                      key={book.path}
                      className={`group relative flex items-start gap-3 rounded-[var(--nova-radius)] border px-3 py-3 text-xs transition-colors ${
                        isCurrent
                          ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.04)]'
                          : 'border-transparent bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)]'
                      }`}
                    >
                      {isCurrent && (
                        <div className="absolute left-0 top-2 bottom-2 w-[3px] rounded-r bg-[var(--nova-accent)]" />
                      )}
                      <button
                        type="button"
                        className="min-w-0 flex-1 pl-1 text-left"
                        onClick={() => handleSwitch(book.path)}
                      >
                        <div className="truncate text-sm font-semibold text-[var(--nova-text)]">{book.name || '未命名书籍'}</div>
                        <div className="mt-1 flex min-w-0 flex-wrap items-center gap-2 text-[11px] text-[var(--nova-text-faint)]">
                          {book.author && <span>{book.author}</span>}
                          {book.last_opened_at && <span>{relativeTime(book.last_opened_at)}</span>}
                          {isCurrent && <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[var(--nova-text-muted)]">当前</span>}
                        </div>
                        <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">{book.path}</div>
                      </button>
                      <div className="flex shrink-0 items-center gap-0.5 pt-0.5">
                        <TooltipIconButton
                          label="编辑信息"
                          className={`${iconButtonCls} opacity-0 group-hover:opacity-100`}
                          onClick={() => startEdit(book)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </TooltipIconButton>
                        <TooltipIconButton
                          label="移除记录"
                          className="nova-nav-item text-[var(--nova-text-faint)] opacity-0 hover:bg-red-500/15 hover:text-red-200 group-hover:opacity-100"
                          onClick={() => handleRemove(book.path)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </TooltipIconButton>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </section>

        </div>
      </ScrollArea>
    </div>
  )
}
