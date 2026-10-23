import { execFileSync } from 'node:child_process'
import { appendFileSync } from 'node:fs'
import { pathToFileURL } from 'node:url'

// Unknown paths require product verification. Only independent documentation,
// promotional images and the separately verified evaluation tool are excluded.
const independent = /^(docs\/|img\/|tools\/creative-eval\/|scripts\/eval-creative\.sh$|(?:README(?:\.en)?|AGENTS|CHANGELOG|CONTRIBUTING)\.md$|LICENSE$|\.git(?:ignore|attributes)$)/
const manifests = /^(agent\/)?go\.(mod|sum)$/
const updater = /^(internal\/(update|hostruntime|localfs|portablepath|buildinfo)\/|cmd\/denova-updater\/)/

export function planCI(files, full) {
  const workflow = files.some(file => file.startsWith('.github/'))
  const dependencies = files.some(file => manifests.test(file))
  const code = full || files.some(file => !independent.test(file))
  // Frontend and workflow changes need every browser journey, not just smoke:
  // most regressions reach production through the UI, and a smoke-only run
  // leaves the remaining journeys unverified until a release tag is pushed.
  const browserFull = full || workflow || files.some(file => file.startsWith('web/'))
  const modules = []
  if (full || workflow || dependencies || files.some(file => !independent.test(file) && !file.startsWith('web/'))) modules.push('.')
  if (full || workflow || dependencies || files.some(file => file.startsWith('agent/'))) modules.push('agent')
  return {
    code,
    full,
    browser_full: browserFull,
    manifests: full || dependencies,
    go_modules: modules,
    unit: full || workflow || files.some(file => file.startsWith('web/')),
    updater: full || workflow || dependencies || files.some(file => updater.test(file)),
    creative_eval: full || workflow || files.some(file => /^(tools\/creative-eval\/|scripts\/eval-creative\.sh$)/.test(file)),
    browser_shards: browserFull ? [1, 2, 3] : [1],
    shard_count: browserFull ? 3 : 1,
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const full = process.env.GITHUB_EVENT_NAME === 'schedule'
    || process.env.GITHUB_EVENT_NAME === 'workflow_dispatch' && process.env.CI_SUITE === 'full'
  let base = process.env.BASE
  if (process.env.GITHUB_EVENT_NAME === 'push') {
    // A new push can cancel its predecessor before the selected suites pass.
    // Compare trees with the last green push, not just the preceding commit,
    // so a subsequent docs/frontend change cannot hide unverified Go changes.
    const url = new URL(`${process.env.GITHUB_API_URL}/repos/${process.env.GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs`)
    url.search = new URLSearchParams({ branch: process.env.GITHUB_REF_NAME, event: 'push', status: 'success', per_page: '1' }).toString()
    const response = await fetch(url, {
      headers: { Accept: 'application/vnd.github+json', Authorization: `Bearer ${process.env.GH_TOKEN}` },
      signal: AbortSignal.timeout(20_000),
    })
    if (!response.ok) throw new Error(`Failed to read the last verified CI commit: HTTP ${response.status}`)
    const history = await response.json()
    base = history.workflow_runs[0]?.head_sha
  }
  console.log('CI comparison baseline:', base || 'all responsibilities')
  let files
  if (full || !base || /^0+$/.test(base)) {
    execFileSync('git', ['show', '--format=', '--check', 'HEAD'], { stdio: 'inherit' })
    // A new ref or explicit smoke dispatch has no comparison base. Verify all
    // product responsibilities without upgrading the requested browser suite.
    files = ['.github/workflows/ci.yml']
  } else {
    try {
      execFileSync('git', ['cat-file', '-e', base], { stdio: 'ignore' })
    } catch {
      execFileSync('git', ['fetch', '--depth=1', 'origin', base], { stdio: 'inherit' })
    }
    execFileSync('git', ['diff', '--check', base, 'HEAD'], { stdio: 'inherit' })
    // A rename into docs must still verify removal of the old product path.
    files = execFileSync('git', ['diff', '--no-renames', '--name-only', '-z', base, 'HEAD'], { encoding: 'utf8' }).split('\0').filter(Boolean)
  }
  const plan = planCI(files, full)
  console.log('CI verification plan:', JSON.stringify(plan))
  appendFileSync(process.env.GITHUB_OUTPUT, Object.entries(plan)
    .map(([key, value]) => `${key}=${JSON.stringify(value)}\n`).join(''))
}
