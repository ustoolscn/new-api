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
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { getUncachedInputTokens } from './tokens'

test('Anthropic input remains separate from cache, including the Opus regression', () => {
  assert.equal(
    getUncachedInputTokens(30592, {
      usage_semantic: 'anthropic',
      cache_tokens: 406435,
    }),
    30592
  )
  assert.equal(
    getUncachedInputTokens(12897, {
      usage_semantic: 'anthropic',
      cache_tokens: 11074,
      cache_creation_tokens: 384949,
    }),
    12897
  )
  assert.equal(
    getUncachedInputTokens(0, {
      usage_semantic: 'anthropic',
      cache_tokens: 395149,
    }),
    0
  )
})

test('legacy Claude logs retain exclusive input semantics', () => {
  assert.equal(
    getUncachedInputTokens(100, {
      claude: true,
      cache_tokens: 200,
      cache_creation_tokens: 300,
    }),
    100
  )
})

test('inclusive input excludes cache once, even with both total and TTL counts', () => {
  const cache = {
    cache_tokens: 200,
    cache_creation_tokens: 300,
    cache_creation_tokens_5m: 100,
    cache_creation_tokens_1h: 200,
  }
  assert.equal(getUncachedInputTokens(1000, cache), 500)
  assert.equal(
    getUncachedInputTokens(1000, {
      ...cache,
      cache_creation_tokens: undefined,
    }),
    500
  )
  assert.equal(
    getUncachedInputTokens(1000, {
      ...cache,
      usage_semantic: 'openai',
      claude: true,
    }),
    500
  )
})

test('uncached, missing metadata and fully cached inputs', () => {
  assert.equal(getUncachedInputTokens(1000, null), 1000)
  assert.equal(getUncachedInputTokens(0, null), 0)
  assert.equal(getUncachedInputTokens(1000, { cache_tokens: 1000 }), 0)
  assert.equal(getUncachedInputTokens(100, { cache_tokens: 200 }), 0)
})
