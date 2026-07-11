import type {
  ClientIPBlacklistSettingsResponse,
  ClientIPBlacklistSettings,
} from '../types'

export function splitIPRules(value: string): string[] {
  const seen = new Set<string>()
  const rules: string[] = []

  for (const line of value.split('\n')) {
    const rule = line.trim()
    if (!rule || seen.has(rule)) continue
    seen.add(rule)
    rules.push(rule)
  }

  return rules
}

export function requireClientIPBlacklistResponseData(
  response: ClientIPBlacklistSettingsResponse
): ClientIPBlacklistSettingsResponse & { data: ClientIPBlacklistSettings } {
  if (!response.success || !response.data) {
    throw new Error(response.message || 'Failed to update client IP blacklist')
  }
  return response as ClientIPBlacklistSettingsResponse & {
    data: ClientIPBlacklistSettings
  }
}
