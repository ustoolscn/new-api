import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  shouldRefreshNoticeForOption,
  shouldRefreshStatusForOption,
} from './option-refresh'

describe('system option status refresh', () => {
  test('refreshes status when system announcements change', () => {
    assert.equal(
      shouldRefreshStatusForOption('console_setting.announcements'),
      true
    )
    assert.equal(
      shouldRefreshStatusForOption('console_setting.announcements_enabled'),
      true
    )
  })

  test('does not refresh status for settings that are not status-backed', () => {
    assert.equal(shouldRefreshStatusForOption('console_setting.faq'), false)
  })

  test('refreshes notice query only when the system notice changes', () => {
    assert.equal(shouldRefreshNoticeForOption('Notice'), true)
    assert.equal(shouldRefreshNoticeForOption('console_setting.announcements'), false)
  })
})
