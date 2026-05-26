import { useEffect, useState } from 'react'
import { BookMarked, FileText, Globe2, UserRound } from 'lucide-react'
import { readFile, saveFile } from '@/lib/api'
import { isSaveShortcut } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

const FILES = [
  { label: '角色', path: 'setting/characters.md', icon: UserRound },
  { label: '世界观', path: 'setting/world-building.md', icon: Globe2 },
  { label: 'CREATOR', path: 'CREATOR.md', icon: BookMarked },
]

export function SettingPanel() {
  const [active, setActive] = useState(FILES[0].path)
  const [content, setContent] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let cancelled = false
    readFile(active)
      .then((data) => { if (!cancelled) setContent(data.content) })
      .catch(() => { if (!cancelled) setContent('') })
    return () => { cancelled = true }
  }, [active])

  const handleSave = async () => {
    setSaving(true)
    try {
      await saveFile(active, content)
    } finally {
      setSaving(false)
    }
  }

  return (
    <aside className="flex h-full min-w-0 flex-col border-r border-[#2f3540] bg-[#1b1e24] p-4">
      <div className="mb-3 flex h-8 items-center justify-between">
        <div>
          <div className="text-sm font-semibold text-[#e0e4ec]">资料库</div>
          <div className="text-[11px] text-[#7f8898]">角色、世界观与创作者规则</div>
        </div>
        <FileText className="h-4 w-4 text-[#7aa2d8]" />
      </div>
      <div className="mb-3">
        <Tabs value={active} onValueChange={setActive}>
          <TabsList className="h-auto min-h-9 w-full flex-wrap bg-[#252a33]">
            {FILES.map((file) => {
              const Icon = file.icon
              return (
                <TabsTrigger key={file.path} value={file.path} className="min-w-0 flex-1 gap-1.5 text-xs">
                  <Icon className="h-3.5 w-3.5" />
                  {file.label}
                </TabsTrigger>
              )
            })}
          </TabsList>
        </Tabs>
      </div>
      <SettingOverview content={content} />
      <ScrollArea className="min-h-0 flex-1 rounded-xl border border-[#343b47] bg-[#111318]">
        <Textarea
          className="min-h-[calc(100vh-350px)] resize-none border-0 bg-transparent font-mono text-xs leading-6 text-[#b6bdca] shadow-none focus-visible:ring-0"
          value={content}
          onChange={(event) => setContent(event.target.value)}
          onKeyDown={(event) => {
            if (isSaveShortcut(event)) {
              event.preventDefault()
              event.stopPropagation()
              void handleSave()
            }
          }}
        />
      </ScrollArea>
      <div className="pt-3">
        <Button className="w-full border-[#343b47] bg-[#242a33] text-[#d7dbe2] hover:bg-[#2b3340]" variant="outline" size="sm" disabled={saving} onClick={handleSave}>
          {saving ? '保存中...' : '保存设定'}
        </Button>
      </div>
    </aside>
  )
}

function SettingOverview({ content }: { content: string }) {
  const sections = extractSections(content)
  if (sections.length === 0) {
    return (
      <div className="mb-3 rounded-lg border border-dashed border-[#343b47] bg-[#141820] px-3 py-2 text-xs leading-5 text-[#7f8898]">
        暂无可展示的资料摘要
      </div>
    )
  }

  return (
    <div className="mb-3 grid gap-2">
      {sections.slice(0, 3).map((section) => (
        <div key={section.title} className="rounded-lg border border-[#343b47] bg-[#141820] p-3">
          <div className="mb-1 truncate text-xs font-semibold text-[#dce3ee]">{section.title}</div>
          <div className="line-clamp-2 text-[11px] leading-5 text-[#8d96a7]">{section.summary}</div>
        </div>
      ))}
    </div>
  )
}

function extractSections(content: string) {
  const lines = content.split('\n')
  const sections: { title: string; summary: string }[] = []
  let currentTitle = ''
  let body: string[] = []

  const push = () => {
    const summary = body
      .map((line) => line.replace(/^[-*]\s*/, '').trim())
      .filter(Boolean)
      .slice(0, 3)
      .join(' / ')
    if (currentTitle && summary) sections.push({ title: currentTitle, summary })
  }

  for (const line of lines) {
    const heading = line.match(/^#{1,3}\s+(.+)$/)
    if (heading) {
      push()
      currentTitle = heading[1].trim()
      body = []
      continue
    }
    if (currentTitle) body.push(line)
  }
  push()
  return sections
}
