import { spawn, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const repositoryRoot = path.resolve(webRoot, '..')
const testResultsRoot = path.join(webRoot, 'test-results')
const runtimeRoot = path.join(testResultsRoot, 'runtime')
const relativeRuntime = path.relative(testResultsRoot, runtimeRoot)

if (relativeRuntime.startsWith('..') || path.isAbsolute(relativeRuntime)) {
  throw new Error(`Refusing to prepare E2E runtime outside ${testResultsRoot}`)
}

if (existsSync(runtimeRoot)) rmSync(runtimeRoot, { recursive: true, force: true })
const denovaDir = path.join(runtimeRoot, 'denova')
const binaryDir = path.join(runtimeRoot, 'bin')
mkdirSync(denovaDir, { recursive: true })
mkdirSync(binaryDir, { recursive: true })

const backendPort = process.env.DENOVA_E2E_BACKEND_PORT || '18080'
const modelPort = process.env.DENOVA_E2E_MODEL_PORT || '18081'
const binaryPath = path.join(binaryDir, process.platform === 'win32' ? 'denova-e2e.exe' : 'denova-e2e')
const config = `language = "zh-CN"
update_check_enabled = false
model_max_retries = 1

[[model_endpoints]]
id = "e2e"
name = "E2E deterministic model"
provider = "openai-compatible"
protocol = "openai-chat-completions"
api_key = "e2e-test-key"
base_url = "http://127.0.0.1:${modelPort}/v1"

[[model_profiles]]
id = "e2e"
name = "E2E deterministic model"
endpoint_id = "e2e"
model = "denova-e2e"
context_window_tokens = 100000

[agent_models.default]
profile_id = "e2e"
thinking_level = "off"

[agent_models.ide]
profile_id = "e2e"
thinking_level = "off"

[agent_models.interactive_story]
profile_id = "e2e"
thinking_level = "off"

[agent_models.interactive_director]
profile_id = "e2e"
thinking_level = "off"
`
writeFileSync(path.join(denovaDir, 'config.toml'), config, 'utf8')

const build = spawnSync('go', ['build', '-o', binaryPath, './cmd/denova'], {
  cwd: repositoryRoot,
  env: process.env,
  stdio: 'inherit',
})
if (build.error) throw build.error
if (build.status !== 0) process.exit(build.status ?? 1)

const backend = spawn(binaryPath, ['--no-open', `--port=${backendPort}`], {
  cwd: runtimeRoot,
  env: {
    ...process.env,
    DENOVA_DIR: denovaDir,
    DENOVA_SKILLS_DIR: path.join(repositoryRoot, 'skills'),
  },
  stdio: 'inherit',
})

function forwardSignal(signal) {
  if (!backend.killed) backend.kill(signal)
}
process.once('SIGINT', () => forwardSignal('SIGINT'))
process.once('SIGTERM', () => forwardSignal('SIGTERM'))
backend.once('error', (error) => {
  console.error('[e2e-backend] failed to start', error)
  process.exitCode = 1
})
backend.once('exit', (code) => process.exit(code ?? 0))
