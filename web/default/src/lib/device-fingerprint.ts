const DEVICE_ID_STORAGE_KEY = 'new-api-device-id'

let fingerprintPromise: Promise<string | null> | null = null

function getOrCreateDeviceId(): string | null {
  if (typeof window === 'undefined') return null

  try {
    const stored = window.localStorage.getItem(DEVICE_ID_STORAGE_KEY)
    if (stored) return stored

    const deviceId =
      typeof window.crypto.randomUUID === 'function'
        ? window.crypto.randomUUID()
        : [...window.crypto.getRandomValues(new Uint8Array(16))]
            .map((byte) => byte.toString(16).padStart(2, '0'))
            .join('')
    window.localStorage.setItem(DEVICE_ID_STORAGE_KEY, deviceId)
    return deviceId
  } catch {
    return null
  }
}

async function hashFingerprint(value: string): Promise<string> {
  const digest = await crypto.subtle.digest(
    'SHA-256',
    new TextEncoder().encode(value)
  )
  return [...new Uint8Array(digest)]
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}

export function getDeviceFingerprint(): Promise<string | null> {
  if (fingerprintPromise) return fingerprintPromise

  fingerprintPromise = (async () => {
    const deviceId = getOrCreateDeviceId()
    if (!deviceId || typeof window === 'undefined') return null

    const source = `v1|${deviceId}`

    try {
      return await hashFingerprint(source)
    } catch {
      return deviceId
    }
  })()

  return fingerprintPromise
}
