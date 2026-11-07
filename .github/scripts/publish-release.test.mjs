import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test } from 'node:test'

const script = fileURLToPath(new URL('./publish-release.sh', import.meta.url))
const tag = 'v0.7.2'
const updatesTag = 'desktop-updates-v0.7.2'
const sha = 'a'.repeat(40)
const downloads = [
  `denova-desktop-${tag}-macos-arm64.dmg`,
  `denova-desktop-${tag}-windows-x64-setup.exe`,
  ...['macos-arm64', 'macos-x64', 'linux-arm64', 'linux-x64', 'windows-x64'].map(target =>
    `denova-webui-${tag}-${target}.${target === 'windows-x64' ? 'zip' : 'tar.gz'}`),
]

// Model only the release API boundary. Exercise the real inventory validation,
// checksums, draft ordering, explicit uploads, and recovery against local files.
const fakeGh = `#!/usr/bin/env node
const fs = require('node:fs')
const crypto = require('node:crypto')
const [kind, command, tag, ...args] = process.argv.slice(2)
if (kind !== 'release') process.exit(2)
const file = process.env.RELEASE_TEST_STATE
const state = JSON.parse(fs.readFileSync(file, 'utf8'))
const release = state.releases[tag]
state.calls.push([command, tag, ...args])
function save() { fs.writeFileSync(file, JSON.stringify(state)) }
function assets(files) {
  return files.map(name => ({ name, digest: 'sha256:' + crypto.createHash('sha256').update(fs.readFileSync(name)).digest('hex') }))
}
switch (command) {
  case 'view':
    save()
    if (!release) process.exit(1)
    if (args.includes('isDraft')) console.log(release.draft)
    else for (const asset of release.assets) console.log(asset.name + '\\t' + asset.digest)
    break
  case 'create':
    if (release) process.exit(1)
    state.releases[tag] = { draft: true, latest: false, assets: assets(args.slice(0, args.indexOf('--draft'))), sha: args[args.indexOf('--target') + 1] }
    save()
    break
  case 'upload':
    if (!release || !release.draft) process.exit(1)
    const uploaded = assets(args.slice(0, args.indexOf('--clobber')))
    release.assets = release.assets.filter(a => !uploaded.some(b => b.name === a.name)).concat(uploaded)
    save()
    break
  case 'edit':
    if (!release) process.exit(1)
    if (args.includes('--draft=false')) release.draft = false
    if (args.includes('--latest=false')) release.latest = false
    if (args.includes('--latest')) release.latest = true
    save()
    break
  default: process.exit(2)
}
`

function fixture(t) {
  const directory = mkdtempSync(path.join(os.tmpdir(), 'denova-publish-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  mkdirSync(path.join(directory, 'bin'))
  writeFileSync(path.join(directory, 'bin', 'gh'), fakeGh, { mode: 0o755 })
  const statePath = path.join(directory, 'state.json')
  const state = () => JSON.parse(readFileSync(statePath, 'utf8'))
  const save = value => writeFileSync(statePath, JSON.stringify(value))
  save({ releases: {}, calls: [] })
  for (const name of downloads) writeFileSync(path.join(directory, name), `fixture ${name}\n`)
  for (const target of ['darwin-arm64', 'windows-amd64']) {
    const archive = `denovadesktop-0.7.2-${target}.tar.gz`
    const content = Buffer.from(`update fixture ${target}\n`)
    writeFileSync(path.join(directory, archive), content)
    writeFileSync(path.join(directory, `update-${target}.json`), JSON.stringify({
      version: '0.7.2', url: `https://github.com/example/denova/releases/download/${updatesTag}/${archive}`,
      size: content.length, signature: 'fixture signature',
    }))
  }
  writeFileSync(path.join(directory, 'RELEASE_NOTES.md'), 'Release fixture\n')
  writeFileSync(path.join(directory, 'install.sh'), 'Unrelated staged installer\n')
  writeFileSync(path.join(directory, 'denova-v0.7.2-darwin-arm64.tar.gz'), 'Unrelated legacy copy\n')
  const run = () => execFileSync('bash', [script, tag, sha], {
    cwd: directory, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, PATH: `${path.join(directory, 'bin')}:${process.env.PATH}`, GH_REPO: 'example/denova', RELEASE_TEST_STATE: statePath },
  })
  return { directory, state, save, run }
}

test('publishes exactly eight primary assets and four separate update assets', t => {
  const f = fixture(t)
  f.run()
  const { releases, calls } = f.state()
  assert.deepEqual(releases[tag].assets.map(a => a.name).sort(), [...downloads, 'checksums.txt'].sort())
  assert.equal(releases[updatesTag].assets.length, 4)
  assert.equal(releases[tag].latest, true)
  assert.equal(releases[updatesTag].latest, false)
  assert.equal(releases[tag].draft, false)
  assert.equal(releases[updatesTag].draft, false)
  assert.equal(releases[tag].sha, sha)
  const published = calls.filter(call => call.includes('--draft=false'))
  assert.deepEqual(published.map(call => call[1]), [updatesTag, tag])
  const checksums = readFileSync(path.join(f.directory, 'checksums.txt'), 'utf8').trim().split('\n')
  assert.equal(checksums.length, 7)
  for (const name of downloads) {
    const sum = createHash('sha256').update(readFileSync(path.join(f.directory, name))).digest('hex')
    assert.ok(checksums.includes(`${sum}  ${name}`))
  }
  const mutations = calls.filter(call => ['create', 'upload'].includes(call[0])).length
  f.run()
  assert.equal(f.state().calls.filter(call => ['create', 'upload'].includes(call[0])).length, mutations)
})

test('rejects an extra remote attachment before either release is published', t => {
  const f = fixture(t)
  f.save({ releases: { [tag]: { draft: true, assets: [{ name: 'install.sh', digest: 'sha256:unexpected' }] } }, calls: [] })
  assert.throws(f.run, /unexpected assets or checksums/)
  assert.ok(Object.values(f.state().releases).every(release => release.draft))
})

test('never replaces conflicting bytes in an already published release', t => {
  const f = fixture(t)
  f.run()
  writeFileSync(path.join(f.directory, downloads[0]), 'Changed installer\n')
  const before = f.state().calls.length
  assert.throws(f.run, /unexpected assets or checksums/)
  assert.ok(f.state().calls.slice(before).every(call => call[0] === 'view'))
})

test('rejects an update manifest pointing at the primary download release', t => {
  const f = fixture(t)
  const manifestPath = path.join(f.directory, 'update-darwin-arm64.json')
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'))
  manifest.url = manifest.url.replace(updatesTag, tag)
  writeFileSync(manifestPath, JSON.stringify(manifest))
  assert.throws(f.run)
  assert.equal(f.state().calls.length, 0)
})
