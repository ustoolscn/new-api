import assert from 'node:assert/strict'
import { beforeEach, describe, test } from 'node:test'
import { STORAGE_KEYS } from '../constants'
import { clearStoredMessages, loadMessages, saveConfig, saveMessages } from './storage'

class LocalStorageMock {
  private store = new Map<string, string>()

  get length() {
    return this.store.size
  }

  getItem(key: string) {
    return this.store.get(key) ?? null
  }

  key(index: number) {
    return Array.from(this.store.keys())[index] ?? null
  }

  setItem(key: string, value: string) {
    this.store.set(key, value)
  }

  removeItem(key: string) {
    this.store.delete(key)
  }

  clear() {
    this.store.clear()
  }
}

describe('playground storage', () => {
  beforeEach(() => {
    globalThis.localStorage = new LocalStorageMock() as Storage
  })

  test('clears only stored chat messages', () => {
    saveConfig({ model: 'gpt-4o' })
    saveMessages([
      {
        key: 'user-message',
        from: 'user',
        versions: [{ id: 'version-1', content: 'hello' }],
      },
    ])

    clearStoredMessages()

    assert.equal(localStorage.getItem(STORAGE_KEYS.MESSAGES), null)
    assert.equal(loadMessages(), null)
    assert.equal(
      localStorage.getItem(STORAGE_KEYS.CONFIG),
      JSON.stringify({ model: 'gpt-4o' })
    )
  })
})
