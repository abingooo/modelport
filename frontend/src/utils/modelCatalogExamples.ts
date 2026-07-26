import type { ModelCatalogItem, ModelInterfaceFormat } from '@/api/modelCatalog'

function stripKnownEndpoint(value: string): string {
  return value.replace(
    /\/(?:v1\/chat\/completions|v1\/messages|v1\/embeddings|v1\/images\/(?:generations|edits)|v1\/videos\/(?:generations|edits|extensions)|v1beta\/models\/[^/]+:(?:generateContent|streamGenerateContent))\/?$/i,
    '',
  )
}

export function normalizeCatalogGatewayBase(value?: string): string {
  const fallback = typeof window === 'undefined' ? 'https://modelport.link' : window.location.origin
  const raw = stripKnownEndpoint((value || fallback).trim().replace(/\/+$/, ''))
  return raw.replace(/\/(?:v1|v1beta)\/?$/i, '')
}

function jsonBody(value: Record<string, unknown>): string {
  return JSON.stringify(value, null, 2)
}

function curlCommand(endpoint: string, headers: string[], body: Record<string, unknown>): string {
  const continuation = String.fromCharCode(92)
  const lines = [
    `curl --request POST '${endpoint}' ${continuation}`,
    ...headers.map((header) => `  -H '${header}' ${continuation}`),
    `  --data '${jsonBody(body)}'`,
  ]
  return lines.join('\n')
}

function primaryScenario(item: ModelCatalogItem): string {
  return item.scenarios[0] || 'chat'
}

function openAIExample(item: ModelCatalogItem, base: string): string {
  const scenario = primaryScenario(item)
  let path = '/v1/chat/completions'
  let body: Record<string, unknown> = {
    model: item.name,
    messages: [{ role: 'user', content: 'Hello from ModelPort' }],
    stream: true,
  }
  if (scenario === 'embedding') {
    path = '/v1/embeddings'
    body = { model: item.name, input: 'Hello from ModelPort' }
  } else if (scenario === 'image') {
    path = '/v1/images/generations'
    body = { model: item.name, prompt: 'A cargo ship entering a modern model harbor', size: '1024x1024' }
  } else if (scenario === 'video') {
    path = '/v1/videos/generations'
    body = { model: item.name, prompt: 'A cargo ship entering a modern model harbor' }
  }
  return curlCommand(`${base}${path}`, [
    'Authorization: Bearer $MODELPORT_API_KEY',
    'Content-Type: application/json',
  ], body)
}

function anthropicExample(item: ModelCatalogItem, base: string): string {
  return curlCommand(`${base}/v1/messages`, [
    'x-api-key: $MODELPORT_API_KEY',
    'anthropic-version: 2023-06-01',
    'Content-Type: application/json',
  ], {
    model: item.name,
    max_tokens: 1024,
    messages: [{ role: 'user', content: 'Hello from ModelPort' }],
  })
}

function googleExample(item: ModelCatalogItem, base: string): string {
  const endpoint = `${base}/v1beta/models/${encodeURIComponent(item.name)}:generateContent`
  return curlCommand(endpoint, [
    'x-goog-api-key: $MODELPORT_API_KEY',
    'Content-Type: application/json',
  ], {
    contents: [{ role: 'user', parts: [{ text: 'Hello from ModelPort' }] }],
  })
}

export function buildModelCatalogExample(
  item: ModelCatalogItem,
  format: ModelInterfaceFormat,
  configuredBaseUrl?: string,
): string {
  const override = item.example_overrides?.[format]?.trim()
  if (override) return override
  const base = normalizeCatalogGatewayBase(configuredBaseUrl)
  switch (format) {
    case 'anthropic':
      return anthropicExample(item, base)
    case 'google':
      return googleExample(item, base)
    default:
      return openAIExample(item, base)
  }
}
