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

import { channelSchema } from '../types'
import {
  channelFormSchema,
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToUpdatePayload,
} from './channel-form'

test('channel usage adjustment defaults off and round-trips without changing other settings', () => {
  const rule = {
    enabled: true,
    cache_min: -30,
    cache_max: -10,
    output_min: 0,
    output_max: 10,
  }
  const channel = channelSchema.parse({
    id: 148,
    name: 'test',
    type: 14,
    key: '',
    status: 1,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    setting: JSON.stringify({
      force_format: true,
      system_prompt: 'unchanged',
      usage_adjustment: rule,
    }),
  })
  assert.equal(CHANNEL_FORM_DEFAULT_VALUES.usage_adjustment?.enabled, false)
  const form = transformChannelToFormDefaults(channel)
  assert.deepEqual(form.usage_adjustment, rule)
  const payload = transformFormDataToUpdatePayload(form, channel.id)
  const saved = JSON.parse(payload.setting ?? '{}')
  assert.deepEqual(saved.usage_adjustment, rule)
  assert.equal(saved.force_format, true)
  assert.equal(saved.system_prompt, 'unchanged')
  assert.equal(
    transformChannelToFormDefaults({ ...channel, setting: '{}' })
      .usage_adjustment?.enabled,
    false
  )
})

test('channel form rejects reversed or excessive adjustment bounds', () => {
  const valid = {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'test',
    models: 'test',
    key: 'test',
    usage_adjustment: {
      enabled: true,
      cache_min: -30,
      cache_max: -10,
      output_min: 0,
      output_max: 10,
    },
  }
  assert.equal(channelFormSchema.safeParse(valid).success, true)
  for (const change of [
    { cache_min: 0 },
    { output_max: 101 },
    { output_min: -101 },
    { cache_max: Number.NaN },
  ]) {
    assert.equal(
      channelFormSchema.safeParse({
        ...valid,
        usage_adjustment: { ...valid.usage_adjustment, ...change },
      }).success,
      false
    )
  }
})
