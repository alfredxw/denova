/** API 请求模块，处理与后端的通信 */

export interface ChatMessage {
  type?: 'message' | 'clear'
  role?: 'user' | 'assistant' | 'thinking' | 'tool_call' | 'tool_result' | 'system' | 'error'
  content?: string
  id?: string
  turn_id?: string
  name?: string
  args?: string
  status?: 'running' | 'success' | 'error'
  result?: string
  streaming?: boolean
  created_at?: string
}

export interface SessionSummary {
  id: string
  title: string
  created_at: string
  updated_at: string
  active: boolean
  message_count: number
}

export interface SSEEvent {
  event: string
  data: string
}

export interface FileOperationResult {
  path: string
  message: string
}

export interface CreateFileRequest {
  path: string
  type: 'file' | 'dir'
  content?: string
}

export interface CopyMoveRequest {
  from: string
  to: string
}

export interface RenameRequest {
  path: string
  new_name: string
}

export interface BookRecord {
  name: string
  path: string
  author: string
  last_opened_at: string
}

export interface ChapterSummary {
  path: string
  file_name: string
  display_title: string
  index: number
  words: number
  status: string
  updated_at: string
  volume: string
  volume_path: string
}

export interface DocumentPreview {
  path: string
  title: string
  excerpt: string
  words: number
  updated_at: string
}

export interface WorkspaceSummary {
  title: string
  author: string
  chapter_count: number
  total_words: number
  chapters: ChapterSummary[]
  outline?: DocumentPreview
  chapter_plans: DocumentPreview[]
}

export interface CharacterCardImportResult {
  name: string
  target_path: string
  entry_count: number
  item_count: number
  item_ids: string[]
  workspace?: string
  book_meta?: BookMeta
  message: string
}

export interface CharacterCardPreview {
  name: string
  entry_count: number
  tags: string[]
}

/** 书籍元信息 */
export interface BookMeta {
  title: string
  author: string
  description: string
  created_at: string
  updated_at: string
}

export interface GitChange {
  path: string
  status: string
}

export interface GitStatus {
  initialized: boolean
  branch: string
  clean: boolean
  changes: GitChange[]
}

export interface GitCommit {
  hash: string
  short_hash: string
  author: string
  date: string
  subject: string
}

export interface GitCommandResult {
  command: string
  output: string
  status?: GitStatus
}

export interface LoreItem {
  id: string
  type: 'character' | 'world' | 'location' | 'faction' | 'rule' | 'item' | 'other'
  name: string
  importance: 'major' | 'important' | 'minor'
  tags: string[]
  content: string
  created_at: string
  updated_at: string
}

export type LoreItemInput = Omit<LoreItem, 'created_at' | 'updated_at'>

export interface LoreVersion {
  id: string
  message: string
  created_at: string
  item_count: number
}

export interface LoreAgentResult {
  message: string
  version?: LoreVersion
  items: LoreItem[]
  created: LoreItem[]
  updated: LoreItem[]
  deleted_ids: string[]
}

async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  const data = await res.json()
  if (!res.ok) {
    throw new Error(data.error || `HTTP ${res.status}`)
  }
  return data
}

/** 编辑器选中文本引用 */
export interface TextSelection {
  fileName: string
  startLine: number
  endLine: number
  content: string
}

/** 发送消息并返回 SSE 流式 reader */
export async function sendMessage(
  message: string,
  references: string[] = [],
  loreReferences: string[] = [],
  styleReferences: string[] = [],
  textSelections: TextSelection[] = [],
  signal?: AbortSignal,
  planMode?: boolean,
): Promise<ReadableStream<SSEEvent>> {
  const res = await fetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      message,
      references,
      lore_references: loreReferences,
      style_references: styleReferences,
      selections: textSelections.map(s => ({
        file_name: s.fileName,
        start_line: s.startLine,
        end_line: s.endLine,
        content: s.content,
      })),
      plan_mode: planMode || false,
    }),
    signal,
  })

  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')

  return parseSSEStream(res.body)
}

/** 查询当前是否存在后台运行的聊天任务 */
export async function getActiveChatTask(): Promise<{ active: boolean; status?: string }> {
  return requestJSON('/api/chat/active')
}

/** 重新订阅当前后台聊天任务的 SSE 流 */
export async function streamActiveChat(signal?: AbortSignal): Promise<ReadableStream<SSEEvent>> {
  const res = await fetch('/api/chat/stream', { signal })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

/** 终止当前后台聊天任务 */
export async function abortChat(): Promise<void> {
  await requestJSON('/api/chat/abort', { method: 'POST' })
}

/** 获取可用风格参考文件 */
export async function getStyles(): Promise<string[]> {
  const data = await requestJSON<{ styles: string[] }>('/api/styles')
  return data.styles || []
}

/** 解析 SSE 流 */
function parseSSEStream(body: ReadableStream<Uint8Array>): ReadableStream<SSEEvent> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  return new ReadableStream<SSEEvent>({
    async pull(controller) {
      while (true) {
        const { done, value } = await reader.read()
        if (done) {
          controller.close()
          return
        }
        buffer += decoder.decode(value, { stream: true })

        const events = buffer.split('\n\n')
        buffer = events.pop() || ''

        for (const eventStr of events) {
          if (!eventStr.trim()) continue
          const lines = eventStr.split('\n')
          let eventType = ''
          let data = ''
          for (const line of lines) {
            if (line.startsWith('event: ')) eventType = line.slice(7)
            else if (line.startsWith('data: ')) data = line.slice(6)
          }
          if (eventType) {
            controller.enqueue({ event: eventType, data })
          }
        }
      }
    },
  })
}

/** 执行命令 */
export async function executeCommand(command: string): Promise<string> {
  const res = await fetch('/api/command', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ command }),
  })
  const data = await res.json()
  return data.result || ''
}

/** 获取历史消息，sessionId 为空时读取当前激活会话。 */
export async function getMessages(sessionId?: string): Promise<ChatMessage[]> {
  const query = sessionId ? `?session_id=${encodeURIComponent(sessionId)}` : ''
  const res = await fetch(`/api/session/messages${query}`)
  return res.json()
}

/** 获取当前 workspace 下的会话列表。 */
export async function getSessions(): Promise<SessionSummary[]> {
  const data = await requestJSON<{ sessions: SessionSummary[] }>('/api/sessions')
  return data.sessions || []
}

/** 创建并激活新会话。 */
export async function createSession(title?: string): Promise<SessionSummary> {
  return requestJSON('/api/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title: title ?? '' }),
  })
}

/** 切换当前激活会话。 */
export async function switchSession(id: string): Promise<SessionSummary> {
  return requestJSON('/api/sessions/switch', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  })
}

/** 重命名会话标题。 */
export async function renameSession(id: string, title: string): Promise<void> {
  await requestJSON('/api/sessions/rename', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, title }),
  })
}

/** 删除会话，后端会返回新的激活会话。 */
export async function deleteSession(id: string): Promise<SessionSummary> {
  return requestJSON('/api/sessions/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  })
}

/** 获取状态 */
export async function getStatus(): Promise<{ has_state: boolean; context: string }> {
  const res = await fetch('/api/status')
  return res.json()
}

/** 切换 workspace */
export async function switchWorkspace(path: string): Promise<{ workspace: string; message: string }> {
  return requestJSON('/api/workspace/switch', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
}

/** 获取最近打开的书籍列表 */
export async function getBooks(): Promise<BookRecord[]> {
  const data = await requestJSON<{ books: BookRecord[] }>('/api/books')
  return data.books || []
}

/** 移除书籍记录，不删除磁盘目录 */
export async function removeBook(path: string): Promise<{ message: string }> {
  return requestJSON('/api/books/remove', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
}

/** 获取当前 workspace */
export async function getCurrentWorkspace(): Promise<{ workspace: string; has_state: boolean }> {
  return requestJSON('/api/workspace/current')
}

/** 获取当前作品章节统计 */
export async function getWorkspaceSummary(): Promise<WorkspaceSummary> {
  const summary = await requestJSON<WorkspaceSummary>('/api/workspace/summary')
  return {
    ...summary,
    chapters: Array.isArray(summary.chapters) ? summary.chapters : [],
    chapter_plans: Array.isArray(summary.chapter_plans) ? summary.chapter_plans : [],
  }
}

/** 读取文件内容 */
export async function readFile(path: string): Promise<{ path: string; content: string }> {
  return requestJSON(`/api/workspace/file?path=${encodeURIComponent(path)}`)
}

/** 保存文件内容 */
export async function saveFile(path: string, content: string): Promise<{ message: string }> {
  return requestJSON('/api/workspace/file', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, content }),
  })
}

export async function getLoreItems(): Promise<LoreItem[]> {
  const data = await requestJSON<{ items: LoreItem[] }>('/api/lore/items')
  return data.items || []
}

export async function createLoreItem(item: Partial<LoreItemInput>): Promise<LoreItem> {
  return requestJSON('/api/lore/items', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(item),
  })
}

export async function updateLoreItem(id: string, item: Partial<LoreItemInput>): Promise<LoreItem> {
  return requestJSON(`/api/lore/items/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(item),
  })
}

export async function deleteLoreItem(id: string): Promise<void> {
  await requestJSON(`/api/lore/items/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function runLoreAgent(instruction: string, references: string[] = []): Promise<LoreAgentResult> {
  return requestJSON('/api/lore/agent', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ instruction, references }),
  })
}

export async function runLoreAgentStream(instruction: string, references: string[] = []): Promise<ReadableStream<SSEEvent>> {
  const res = await fetch('/api/lore/agent/stream', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ instruction, references }),
  })
  if (!res.ok) {
    let message = `HTTP ${res.status}`
    try {
      const data = await res.json()
      message = data.error || message
    } catch {
      // keep HTTP fallback
    }
    throw new Error(message)
  }
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function getLoreAgentMessages(): Promise<ChatMessage[]> {
  return requestJSON('/api/lore/agent/messages')
}

export async function clearLoreAgentSession(): Promise<void> {
  await requestJSON('/api/lore/agent/clear', { method: 'POST' })
}

export async function getLoreVersions(): Promise<LoreVersion[]> {
  const data = await requestJSON<{ versions: LoreVersion[] }>('/api/lore/versions')
  return data.versions || []
}

export async function createLoreVersion(message: string): Promise<LoreVersion> {
  return requestJSON('/api/lore/versions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  })
}

export async function restoreLoreVersion(id: string): Promise<LoreItem[]> {
  const data = await requestJSON<{ items: LoreItem[] }>(`/api/lore/versions/${encodeURIComponent(id)}/restore`, {
    method: 'POST',
  })
  return data.items || []
}

/** 新建文件或目录 */
export async function createWorkspaceItem(req: CreateFileRequest): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/create', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
}

/** 删除文件或目录 */
export async function deleteWorkspaceItem(path: string): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
}

/** 重命名文件或目录 */
export async function renameWorkspaceItem(req: RenameRequest): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/rename', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
}

/** 复制文件或目录 */
export async function copyWorkspaceItem(req: CopyMoveRequest): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/copy', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
}

/** 移动文件或目录 */
export async function moveWorkspaceItem(req: CopyMoveRequest): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/move', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
}

/** 预览酒馆角色卡 PNG/JSON，不写入资料库 */
export async function previewCharacterCard(file: File): Promise<CharacterCardPreview> {
  const form = new FormData()
  form.append('file', file)
  return requestJSON('/api/workspace/import-character-card/preview', {
    method: 'POST',
    body: form,
  })
}

/** 导入酒馆角色卡 PNG/JSON 到互动资料库 */
export async function importCharacterCard(
  file: File,
  options: { targetMode?: 'current' | 'new_book'; bookTitle?: string } = {},
): Promise<CharacterCardImportResult> {
  const form = new FormData()
  form.append('file', file)
  if (options.targetMode) form.append('target_mode', options.targetMode)
  if (options.bookTitle) form.append('book_title', options.bookTitle)
  return requestJSON('/api/workspace/import-character-card', {
    method: 'POST',
    body: form,
  })
}

/** 新建书籍工作区 */
export async function createBook(title: string, author?: string, description?: string): Promise<{ workspace: string; book_meta: BookMeta }> {
  return requestJSON('/api/books/create', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title, author: author ?? '', description: description ?? '' }),
  })
}

/** 读取书籍元信息 */
export async function getBookInfo(path: string): Promise<BookMeta> {
  return requestJSON(`/api/books/info?path=${encodeURIComponent(path)}`)
}

/** 更新书籍元信息 */
export async function updateBookInfo(path: string, title: string, author: string, description: string): Promise<BookMeta> {
  return requestJSON('/api/books/info', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, title, author, description }),
  })
}

/** 获取当前书籍 Git 状态 */
export async function getGitStatus(): Promise<GitStatus> {
  return requestJSON('/api/git/status')
}

/** 获取当前书籍 Git 提交历史 */
export async function getGitHistory(limit = 20): Promise<GitCommit[]> {
  const data = await requestJSON<{ commits: GitCommit[] }>(`/api/git/history?limit=${encodeURIComponent(String(limit))}`)
  return data.commits || []
}

/** 获取当前书籍 Git diff */
export async function getGitDiff(path?: string): Promise<string> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const data = await requestJSON<{ diff: string }>(`/api/git/diff${query}`)
  return data.diff || ''
}

/** 初始化当前书籍 Git 仓库 */
export async function initGitRepository(): Promise<GitCommandResult> {
  return requestJSON('/api/git/init', { method: 'POST' })
}

/** 创建当前书籍版本 */
export async function createGitVersion(message: string): Promise<GitCommandResult> {
  return requestJSON('/api/git/commit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  })
}

/** 回滚当前书籍到指定版本 */
export async function rollbackGitVersion(hash: string): Promise<GitCommandResult> {
  return requestJSON('/api/git/rollback', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ hash }),
  })
}

/** 暂存当前书籍未提交内容 */
export async function stashGitChanges(): Promise<GitCommandResult> {
  return requestJSON('/api/git/stash', { method: 'POST' })
}

/** 恢复最近一次暂存内容 */
export async function popGitStash(): Promise<GitCommandResult> {
  return requestJSON('/api/git/stash/pop', { method: 'POST' })
}

/** 执行受限 Git 命令 */
export async function runGitCommand(command: string): Promise<GitCommandResult> {
  return requestJSON('/api/git/command', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ command }),
  })
}
