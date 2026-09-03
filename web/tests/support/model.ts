import type { APIResponse } from '@playwright/test'
import { expect, type APIRequestContext } from './fixtures'

export const modelControlURL = `http://127.0.0.1:${process.env.DENOVA_E2E_MODEL_PORT || '18081'}`

export interface E2EModelStatus {
  delayed_waiting: number
  delayed_waiting_by_marker: Record<string, number | undefined>
  request_counts: Record<string, number | undefined>
  game_regeneration_allowed: boolean
  game_regeneration_failure_requests: number
  external_secret_path: string
}

export interface E2EModelRequest {
  messages: Array<{
    role: string
    content?: unknown
    tool_calls?: Array<{
      id?: string
      function?: {
        name?: string
        arguments?: string
      }
    }>
  }>
}

export async function getModelStatus(request: APIRequestContext): Promise<E2EModelStatus> {
  const response = await request.get(`${modelControlURL}/control/status`)
  await expectControlSuccess(response)
  return response.json()
}

export async function getCapturedModelRequest(
  request: APIRequestContext,
  marker: string,
): Promise<E2EModelRequest | null> {
  const response = await request.get(
    `${modelControlURL}/control/captured-request?marker=${encodeURIComponent(marker)}`,
  )
  if (response.status() === 404) return null
  await expectControlSuccess(response)
  return response.json()
}

export async function releaseDelayedRequest(request: APIRequestContext, marker: string): Promise<void> {
  const response = await request.post(`${modelControlURL}/control/release`, { data: { marker } })
  await expectControlSuccess(response)
}

export async function allowGameRegeneration(request: APIRequestContext): Promise<void> {
  const response = await request.post(`${modelControlURL}/control/allow-game-regeneration`)
  await expectControlSuccess(response)
}

async function expectControlSuccess(response: APIResponse): Promise<void> {
  const failureDetails = response.ok() ? undefined : await response.text()
  expect(response.ok(), failureDetails).toBe(true)
}
