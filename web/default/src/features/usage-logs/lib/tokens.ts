/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { LogOtherData } from '../types'

/** Anthropic input excludes cache; OpenAI-compatible input includes it. */
export function getUncachedInputTokens(
  promptTokens: number,
  other: LogOtherData | null
): number {
  const isAnthropic = other?.usage_semantic
    ? other.usage_semantic === 'anthropic'
    : other?.claude === true
  if (isAnthropic) return Math.max(promptTokens, 0)

  // The total and TTL breakdown describe the same cache writes.
  const cacheWrite = Math.max(
    other?.cache_creation_tokens || 0,
    (other?.cache_creation_tokens_5m || 0) +
      (other?.cache_creation_tokens_1h || 0)
  )
  return Math.max(promptTokens - (other?.cache_tokens || 0) - cacheWrite, 0)
}
