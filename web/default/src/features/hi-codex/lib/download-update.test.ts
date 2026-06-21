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
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, test } from 'node:test'

const pageSource = readFileSync(new URL('../index.tsx', import.meta.url), 'utf8')

describe('Hi Codex update metadata loading', () => {
  test('does not ship static update metadata', () => {
    assert.equal(existsSync(resolve('public/hi-codex/update.json')), false)
  })

  test('loads update metadata through the same-origin dynamic proxy', () => {
    assert.match(pageSource, /const UPDATE_ENDPOINT = '\/hi-codex\/update\.json'/)
    assert.doesNotMatch(pageSource, /https:\/\/files\.cooper-api\.com\/hi-codex\/update\.json/)
  })
})
