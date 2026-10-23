import assert from 'node:assert/strict'
import { test } from 'node:test'
import { execFile, execFileSync } from 'node:child_process'
import { mkdtempSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { promisify } from 'node:util'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { planCI } from './ci-plan.mjs'

function createGitFixture(prefix) {
  const directory = mkdtempSync(path.join(os.tmpdir(), prefix))
  const git = (...args) => execFileSync('git', args, { cwd: directory, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
  const commit = () => {
    git('add', '.')
    git('-c', 'user.name=CI verification', '-c', 'user.email=ci@example.invalid', '-c', 'commit.gpgsign=false', 'commit', '-qm', 'Fixture')
    return git('rev-parse', 'HEAD')
  }
  return { directory, git, commit }
}

test('independent changes keep the required check without product jobs', () => {
  const plan = planCI(['img/wechat.png', 'CONTRIBUTING.md', 'tools/creative-eval/run.mjs', 'scripts/eval-creative.sh'], false)
  assert.equal(plan.code, false)
  assert.deepEqual(plan.go_modules, [])
  assert.equal(plan.unit, false)
  assert.equal(plan.updater, false)
  assert.equal(plan.creative_eval, true)
})

test('frontend changes do not run Go or updater suites', () => {
  const plan = planCI(['web/src/App.tsx', 'README.md'], false)
  assert.equal(plan.code, true)
  assert.deepEqual(plan.go_modules, [])
  assert.equal(plan.unit, true)
  assert.equal(plan.updater, false)
  assert.equal(plan.creative_eval, false)
  // Every browser journey is reachable from frontend changes.
  assert.equal(plan.browser_full, true)
  assert.deepEqual(plan.browser_shards, [1, 2, 3])
  assert.equal(plan.shard_count, 3)
})

test('backend-only changes keep the scoped smoke browser run', () => {
  const plan = planCI(['internal/app/chat.go'], false)
  assert.equal(plan.code, true)
  assert.equal(plan.browser_full, false)
  assert.deepEqual(plan.browser_shards, [1])
  assert.equal(plan.shard_count, 1)
})

test('workflow changes verify the whole browser suite they describe', () => {
  const plan = planCI(['.github/workflows/ci.yml'], false)
  assert.equal(plan.browser_full, true)
  assert.deepEqual(plan.browser_shards, [1, 2, 3])
})

test('documentation-only changes do not start the evaluation harness', () => {
  const plan = planCI(['docs/guide.md', 'README.md'], false)
  assert.equal(plan.code, false)
  assert.equal(plan.creative_eval, false)
})

test('Agent and dependency changes also verify the root module consumer', () => {
  for (const file of ['agent/session/session.go', 'agent/go.sum', 'go.mod']) {
    assert.deepEqual(planCI([file], false).go_modules, ['.', 'agent'])
  }
  assert.equal(planCI(['agent/go.sum'], false).updater, true)
})

test('updater dependencies retain native platform verification', () => {
  for (const file of ['internal/update/apply.go', 'internal/localfs/replace.go', 'internal/hostruntime/runtime.go', 'internal/buildinfo/version.go', 'cmd/denova-updater/main.go']) {
    assert.equal(planCI([file], false).updater, true, file)
  }
  assert.equal(planCI(['internal/app/chat.go'], false).updater, false)
  assert.deepEqual(planCI(['internal/app/chat.go'], false).go_modules, ['.'])
})

test('Skills and unknown product paths cannot silently bypass checks', () => {
  for (const file of ['skills/writing/SKILL.md', 'new-product/data.json']) {
    assert.equal(planCI([file], false).code, true, file)
    assert.deepEqual(planCI([file], false).go_modules, ['.'])
  }
})

test('full verification covers every product suite even on a documentation commit', () => {
  const plan = planCI(['docs/guide.md'], true)
  assert.equal(plan.code, true)
  assert.equal(plan.unit, true)
  assert.equal(plan.updater, true)
  assert.equal(plan.manifests, true)
  assert.equal(plan.creative_eval, true)
  assert.deepEqual(plan.go_modules, ['.', 'agent'])
  assert.equal(plan.browser_full, true)
  assert.deepEqual(plan.browser_shards, [1, 2, 3])
})

test('real Git comparison preserves product removals renamed into independent paths', () => {
  const { directory, git, commit } = createGitFixture('denova-ci-plan-')
  try {
    git('init', '-q')
    mkdirSync(path.join(directory, 'internal'))
    mkdirSync(path.join(directory, 'docs'))
    writeFileSync(path.join(directory, 'internal', 'product data.txt'), 'Product fixture\n')
    const base = commit()
    renameSync(path.join(directory, 'internal', 'product data.txt'), path.join(directory, 'docs', 'guide.txt'))
    commit()
    const output = path.join(directory, 'output')
    execFileSync(process.execPath, [fileURLToPath(new URL('./ci-plan.mjs', import.meta.url))], {
      cwd: directory,
      env: { ...process.env, GITHUB_EVENT_NAME: 'pull_request', BASE: base, CI_SUITE: '', GITHUB_OUTPUT: output },
    })
    const plan = Object.fromEntries(readFileSync(output, 'utf8').trim().split('\n').map(line => {
      const separator = line.indexOf('=')
      return [line.slice(0, separator), JSON.parse(line.slice(separator + 1))]
    }))
    assert.equal(plan.code, true)
    assert.deepEqual(plan.go_modules, ['.'])
  } finally {
    rmSync(directory, { recursive: true, force: true })
  }
})

test('a docs push still verifies product changes from a cancelled preceding push', async () => {
  const { directory, git, commit } = createGitFixture('denova-ci-cancelled-')
  let verified
  const requests = []
  const server = createServer((request, response) => {
    requests.push(new URL(request.url, 'http://localhost'))
    response.setHeader('Content-Type', 'application/json')
    response.end(JSON.stringify({ workflow_runs: [{ head_sha: verified }] }))
  })
  try {
    git('init', '-q')
    mkdirSync(path.join(directory, 'internal'))
    writeFileSync(path.join(directory, 'internal', 'product.go'), 'package product\n')
    verified = commit()
    writeFileSync(path.join(directory, 'internal', 'product.go'), 'package changed\n')
    const preceding = commit()
    writeFileSync(path.join(directory, 'README.md'), 'Documentation update\n')
    commit()
    server.listen(0, '127.0.0.1')
    await once(server, 'listening')
    const output = path.join(directory, 'output')
    await promisify(execFile)(process.execPath, [fileURLToPath(new URL('./ci-plan.mjs', import.meta.url))], {
      cwd: directory,
      env: { ...process.env, GITHUB_EVENT_NAME: 'push', BASE: preceding, GITHUB_OUTPUT: output,
        GITHUB_API_URL: `http://127.0.0.1:${server.address().port}`, GITHUB_REPOSITORY: 'example/denova', GITHUB_REF_NAME: 'master', GH_TOKEN: 'fixture' },
    })
    assert.match(readFileSync(output, 'utf8'), /code=true\n/)
    assert.match(readFileSync(output, 'utf8'), /go_modules=\["\."\]\n/)
    assert.equal(requests.length, 1)
    assert.equal(requests[0].searchParams.get('event'), 'push')
    assert.equal(requests[0].searchParams.get('branch'), 'master')
    assert.equal(requests[0].searchParams.get('status'), 'success')
  } finally {
    server.closeAllConnections()
    await new Promise(resolve => server.close(resolve))
    rmSync(directory, { recursive: true, force: true })
  }
})
