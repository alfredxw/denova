/** Rejects a response that could otherwise write data into the wrong mounted Project. */
export function assertProjectScope(expected: string, actual: unknown, resource = 'Project response'): asserts actual is string {
  if (actual !== expected) {
    throw new Error(`${resource} scope mismatch: expected ${expected}, received ${typeof actual === 'string' && actual ? actual : '<empty>'}`)
  }
}
