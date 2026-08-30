/// <reference types="node" />

import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const chatStyles = readFileSync(path.resolve(process.cwd(), 'src/index.css'), 'utf8')

describe('shared Streamdown styles', () => {
  it('applies Denova spacing and block surfaces without requiring the assistant message wrapper', () => {
    expect(chatStyles).toMatch(/(?:^|\r?\n)\.nova-streamdown p \{/)
    expect(chatStyles).toMatch(/(?:^|\r?\n)\.nova-streamdown \[data-streamdown="code-block-body"\] > pre \{/)
    expect(chatStyles).toMatch(/(?:^|\r?\n)\.nova-streamdown :is\(\s*\[data-streamdown="code-block"\],\s*\[data-streamdown="table-wrapper"\]\s*\)/)
  })
})
