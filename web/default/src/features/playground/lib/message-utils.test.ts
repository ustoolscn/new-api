import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { API_ENDPOINTS } from '../constants'
import { createUserMessage, formatMessageForAPI } from './message/message-utils'

describe('playground message image content', () => {
  test('formats uploaded gallery URLs as OpenAI image_url content parts', () => {
    const galleryUrl = 'https://gallery.example/uploads/example.png'
    const message = createUserMessage('What is in this image?', [
      galleryUrl,
    ])

    const apiMessage = formatMessageForAPI(message)

    assert.deepEqual(apiMessage.content, [
      { type: 'text', text: 'What is in this image?' },
      {
        type: 'image_url',
        image_url: { url: galleryUrl },
      },
    ])
  })

  test('uses a playground image upload endpoint before chat requests', () => {
    assert.equal(API_ENDPOINTS.IMAGE_UPLOAD, '/pg/upload-image')
  })
})
