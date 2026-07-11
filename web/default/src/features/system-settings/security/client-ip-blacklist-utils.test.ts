import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { splitIPRules } from './client-ip-blacklist-utils'

describe('splitIPRules', () => {
  test('normalizes multiline rules without changing address text', () => {
    assert.deepEqual(
      splitIPRules(' 203.0.113.7 \n\n2001:db8::/48\n203.0.113.7 '),
      ['203.0.113.7', '2001:db8::/48']
    )
  })
})
