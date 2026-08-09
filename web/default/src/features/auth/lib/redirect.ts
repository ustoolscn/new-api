/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export const OAUTH_REDIRECT_STORAGE_KEY = 'auth:oauth-redirect'

/**
 * Keep redirects inside the current app. Absolute URLs and protocol-relative
 * values are intentionally rejected, including values that contain a
 * backslash which browsers may normalize into a host separator.
 */
export function sanitizeRedirect(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined

  const redirect = value.trim()
  const hasControlCharacter = [...redirect].some((character) => {
    const code = character.charCodeAt(0)
    return code < 32 || code === 127
  })
  if (
    hasControlCharacter ||
    !redirect.startsWith('/') ||
    redirect.startsWith('//') ||
    redirect.includes('\\')
  ) {
    return undefined
  }

  if (typeof window === 'undefined') return redirect

  try {
    const parsed = new URL(redirect, window.location.origin)
    if (parsed.origin !== window.location.origin) return undefined
    return `${parsed.pathname}${parsed.search}${parsed.hash}`
  } catch {
    return undefined
  }
}

/** Convert a router/browser location into the relative URL used by sign-in. */
export function getRelativeLocation(value: unknown): string | undefined {
  if (typeof value !== 'string' || value.trim() === '') return undefined

  const raw = value.trim()
  const relative = sanitizeRedirect(raw)
  if (relative) return relative

  if (typeof window === 'undefined') return undefined

  try {
    const parsed = new URL(raw)
    if (parsed.origin !== window.location.origin) return undefined
    return `${parsed.pathname}${parsed.search}${parsed.hash}`
  } catch {
    return undefined
  }
}

export function saveOAuthRedirect(value: unknown): void {
  if (typeof window === 'undefined') return

  const redirect = sanitizeRedirect(value)
  try {
    if (redirect) {
      window.localStorage.setItem(OAUTH_REDIRECT_STORAGE_KEY, redirect)
    } else {
      window.localStorage.removeItem(OAUTH_REDIRECT_STORAGE_KEY)
    }
  } catch {
    // Ignore storage failures; the login flow still works with the default.
  }
}

export function consumeOAuthRedirect(): string | undefined {
  if (typeof window === 'undefined') return undefined

  try {
    const redirect = sanitizeRedirect(
      window.localStorage.getItem(OAUTH_REDIRECT_STORAGE_KEY)
    )
    window.localStorage.removeItem(OAUTH_REDIRECT_STORAGE_KEY)
    return redirect
  } catch {
    return undefined
  }
}

export function buildSignInPath(value: unknown): string {
  const redirect = sanitizeRedirect(value)
  if (!redirect) return '/sign-in'
  return `/sign-in?redirect=${encodeURIComponent(redirect)}`
}
