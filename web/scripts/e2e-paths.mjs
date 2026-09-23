import path from 'node:path'
import { fileURLToPath } from 'node:url'

export const webRoot = fileURLToPath(new URL('..', import.meta.url))
const testResultsRoot = path.join(webRoot, 'test-results')
// Every server and fixture in a suite must use the same isolated runtime.
export const runtimeRoot = path.resolve(testResultsRoot, process.env.DENOVA_E2E_RUNTIME_DIR || 'runtime')
const relativeRuntime = path.relative(testResultsRoot, runtimeRoot)
if (!relativeRuntime || relativeRuntime.startsWith('..') || path.isAbsolute(relativeRuntime)) {
  throw new Error(`Refusing to prepare E2E runtime outside ${testResultsRoot}`)
}
