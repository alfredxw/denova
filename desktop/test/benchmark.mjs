import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { launch, workspace, until, delay, root } from './driver.mjs'

if (process.platform !== 'darwin') throw new Error('This footprint benchmark requires macOS')
const output = path.join(root, 'output/desktop')
mkdirSync(output, { recursive: true })
const runs = []
function sample(pid, directory) {
  const group = JSON.parse(execFileSync('python3', [path.join(root, 'desktop/test/coalition.py'), String(pid)], { encoding: 'utf8' }))
  assert(group.processes.length <= 15, 'Benchmark inherited another application coalition')
  assert(group.processes.some(p => p.name.includes('WebContent')), 'WebKit content process is missing from memory accounting')
  assert(group.processes.some(p => p.name.endsWith('/backend/denova')), 'Backend is missing from memory accounting')
  const file = path.join(directory, 'memory.json')
  execFileSync('/usr/bin/footprint', [...group.processes.flatMap(p => ['-p', String(p.pid)]), '--noCategories', '-j', file], { stdio: 'ignore' })
  return { ...group, rssBytes: group.processes.reduce((n, p) => n + p.rss, 0), footprintBytes: JSON.parse(readFileSync(file))['total footprint'] }
}
for (let run = 0; run < 3; run++) {
  const directory = workspace('denova-mygo-benchmark-')
  let app
  try {
    const start = performance.now()
    app = await launch(directory)
    await until(() => app.evaluate(() => !!document.querySelector('[aria-label="工作台侧边栏"]')), 60000)
    const startupMs = performance.now() - start
    await delay(3000)
    const idle = sample(app.pid, directory)
    const book = await app.api('/api/books/create', 'POST', { title: 'Desktop benchmark', author: 'Denova' })
    await app.api('/api/workspace/switch', 'POST', { path: book.workspace })
    await app.api(`/api/projects/${book.project_id}/files/operations`, 'POST', { operations: [{ id: 'seed', kind: 'create', path: 'chapters/benchmark.md', type: 'file', content: '# 桌面性能对比\n\n' + '这是相同的长篇写作测试内容。风穿过旧车站，旅人继续记录今天的见闻。\n\n'.repeat(1000) }] })
    await app.evaluate(() => { window.pendingReload = true; location.reload(); return true })
    await until(() => app.evaluate(() => !window.pendingReload && !!document.querySelector('[aria-label="工作台侧边栏"]')))
    await until(() => app.evaluate(() => { const b = [...document.querySelectorAll('button')].find(b => b.textContent.trim() === '写作'); if (!b) return false; b.click(); return true }))
    await until(() => app.evaluate(() => { const b = [...document.querySelectorAll('button')].find(b => b.textContent.trim().startsWith('benchmark')); if (!b) return false; b.click(); return true }))
    await until(() => app.evaluate(() => { const e = document.querySelector('.editor-content .ProseMirror'); return !!e?.getClientRects().length }))
    await delay(3000)
    const document = sample(app.pid, directory)
    await app.screenshot(path.join(output, `benchmark-${run}.png`))
    runs.push({ startupMs, idle, document })
    writeFileSync(path.join(output, 'mygo-benchmark.json'), JSON.stringify({ platform: 'darwin-arm64', mygo: '0.7.4', baselineCommit: 'f26d135c', runs }, null, 2) + '\n')
    console.log(JSON.stringify({ run, startupMs, idleBytes: idle.footprintBytes, documentBytes: document.footprintBytes }))
  } finally { if (app) await app.close(); rmSync(directory, { recursive: true, force: true }) }
}
