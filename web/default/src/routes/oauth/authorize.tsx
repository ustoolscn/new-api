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
import { useQuery } from '@tanstack/react-query'
import {
  createFileRoute,
  redirect,
  useNavigate,
  useSearch,
} from '@tanstack/react-router'
import axios from 'axios'
import { AlertTriangle, Check, ShieldCheck, X } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { PublicLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  getRelativeLocation,
  sanitizeRedirect,
} from '@/features/auth/lib/redirect'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

const searchSchema = z.object({
  response_type: z.string().optional(),
  client_id: z.string().optional(),
  redirect_uri: z.string().optional(),
  state: z.string().optional(),
  scope: z.string().optional(),
  code_challenge: z.string().optional(),
  code_challenge_method: z.string().optional(),
})

type AuthorizeSearch = z.infer<typeof searchSchema>

type AuthorizeUser = {
  username?: string
  display_name?: string
  email?: string
}

type AuthorizationRequest = {
  request_id: string
  client_id: string
  client_name: string
  redirect_uri: string
  scopes: string[] | string
  user?: AuthorizeUser
  expires_at?: string | number
}

type AuthorizationResponse = {
  success: boolean
  message?: string
  data?: AuthorizationRequest
}

function getScopes(scopes: AuthorizationRequest['scopes']): string[] {
  if (Array.isArray(scopes)) return scopes.filter(Boolean)
  return scopes
    .split(/[\s,]+/)
    .map((scope) => scope.trim())
    .filter(Boolean)
}

function formatExpiry(
  value: AuthorizationRequest['expires_at']
): string | null {
  if (value == null || value === '') return null

  const parsed =
    typeof value === 'number' || /^\d+$/.test(String(value))
      ? new Date(Number(value) * 1000)
      : new Date(String(value))
  if (Number.isNaN(parsed.getTime())) return null

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(parsed)
}

function getAuthorizationErrorMessage(
  error: unknown,
  fallback: string
): string {
  if (axios.isAxiosError(error)) {
    const responseData = error.response?.data as
      | { error_description?: unknown; message?: unknown }
      | undefined
    if (typeof responseData?.error_description === 'string') {
      return responseData.error_description
    }
    if (typeof responseData?.message === 'string') {
      return responseData.message
    }
  }

  return error instanceof Error && error.message ? error.message : fallback
}

function AuthorizationError(props: { message: string; title: string }) {
  return (
    <PublicLayout showAuthButtons={false}>
      <div className='mx-auto flex min-h-[calc(100svh-8rem)] w-full max-w-xl items-center justify-center'>
        <Alert variant='destructive'>
          <AlertTriangle />
          <AlertTitle>{props.title}</AlertTitle>
          <AlertDescription>{props.message}</AlertDescription>
        </Alert>
      </div>
    </PublicLayout>
  )
}

function OAuthAuthorize() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const search = useSearch({ from: '/oauth/authorize' }) as AuthorizeSearch
  const requestIsValid =
    search.response_type === 'code' &&
    search.client_id === 'high-codex' &&
    Boolean(search.redirect_uri) &&
    search.scope === 'api' &&
    Boolean(search.state) &&
    Boolean(search.code_challenge) &&
    search.code_challenge_method === 'S256'

  const requestQuery = useQuery({
    queryKey: ['oauth-authorize', search],
    enabled: requestIsValid,
    retry: false,
    queryFn: async () => {
      const response = await api.get<AuthorizationResponse>(
        '/api/oauth/authorize',
        {
          params: search,
          skipBusinessError: true,
          skipErrorHandler: true,
        }
      )
      if (!response.data.success || !response.data.data) {
        throw new Error(
          response.data.message || t('Authorization request failed')
        )
      }
      return response.data.data
    },
  })

  const currentRedirect =
    typeof window !== 'undefined'
      ? getRelativeLocation(
          `${window.location.pathname}${window.location.search}${window.location.hash}`
        )
      : undefined

  useEffect(() => {
    const error = requestQuery.error
    if (!axios.isAxiosError(error) || error.response?.status !== 401) return

    useAuthStore.getState().auth.reset()
    navigate({
      to: '/sign-in',
      search: currentRedirect ? { redirect: currentRedirect } : undefined,
      replace: true,
    })
  }, [currentRedirect, navigate, requestQuery.error])

  if (!requestIsValid) {
    return (
      <AuthorizationError
        title={t('Invalid authorization request')}
        message={t('Missing required authorization parameters.')}
      />
    )
  }

  if (requestQuery.isLoading) {
    return (
      <PublicLayout showAuthButtons={false}>
        <LoadingState message={t('Loading authorization request...')} />
      </PublicLayout>
    )
  }

  if (requestQuery.error || !requestQuery.data) {
    const errorMessage = getAuthorizationErrorMessage(
      requestQuery.error,
      t('This authorization request is invalid or has expired.')
    )
    return (
      <AuthorizationError
        title={t('Authorization request failed')}
        message={errorMessage}
      />
    )
  }

  const request = requestQuery.data
  const scopes = getScopes(request.scopes)
  const userName =
    request.user?.display_name ||
    request.user?.username ||
    request.user?.email ||
    t('Current user')
  const expiry = formatExpiry(request.expires_at)

  return (
    <PublicLayout showAuthButtons={false}>
      <div className='mx-auto flex min-h-[calc(100svh-8rem)] w-full max-w-xl items-center justify-center py-8'>
        <Card className='w-full shadow-sm'>
          <CardHeader className='gap-3 border-b'>
            <div className='bg-primary/10 text-primary flex size-12 items-center justify-center rounded-xl'>
              <ShieldCheck className='size-6' aria-hidden='true' />
            </div>
            <div className='space-y-1'>
              <CardTitle className='text-xl'>
                {request.client_name || t('High Codex')}
              </CardTitle>
              <CardDescription>
                {t('wants to access your account')}
              </CardDescription>
            </div>
          </CardHeader>

          <CardContent className='space-y-6 py-6'>
            <div className='space-y-1'>
              <p className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
                {t('Signed in as')}
              </p>
              <p className='font-medium'>{userName}</p>
            </div>

            <div className='space-y-2'>
              <p className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
                {t('Requested permissions')}
              </p>
              <ul className='space-y-2'>
                {scopes.map((scope) => (
                  <li key={scope} className='flex items-center gap-2 text-sm'>
                    <Check className='text-primary size-4' aria-hidden='true' />
                    <span>
                      {scope === 'api'
                        ? t('Use the API on your behalf')
                        : scope}
                    </span>
                  </li>
                ))}
              </ul>
            </div>

            <div className='bg-muted/50 space-y-1 rounded-lg p-3 text-sm'>
              <p className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
                {t('Callback URL')}
              </p>
              <code className='text-foreground block overflow-x-auto text-xs break-all'>
                {request.redirect_uri}
              </code>
            </div>

            {expiry && (
              <p className='text-muted-foreground text-sm'>
                {t('This request expires at {{time}}.', { time: expiry })}
              </p>
            )}

            <p className='text-muted-foreground text-sm'>
              {t('The application will not receive your password.')}
            </p>
          </CardContent>

          <CardFooter className='flex flex-col-reverse gap-2 sm:flex-row sm:justify-end'>
            <form
              action='/api/oauth/authorize/decision'
              method='post'
              className='w-full sm:w-auto'
            >
              <input
                type='hidden'
                name='request_id'
                value={request.request_id}
              />
              <input type='hidden' name='decision' value='deny' />
              <Button
                type='submit'
                variant='outline'
                className='w-full gap-2 sm:w-auto'
              >
                <X className='size-4' aria-hidden='true' />
                {t('Deny')}
              </Button>
            </form>
            <form
              action='/api/oauth/authorize/decision'
              method='post'
              className='w-full sm:w-auto'
            >
              <input
                type='hidden'
                name='request_id'
                value={request.request_id}
              />
              <input type='hidden' name='decision' value='approve' />
              <Button type='submit' className='w-full gap-2 sm:w-auto'>
                <Check className='size-4' aria-hidden='true' />
                {t('Allow access')}
              </Button>
            </form>
          </CardFooter>
        </Card>
      </div>
    </PublicLayout>
  )
}

export const Route = createFileRoute('/oauth/authorize')({
  validateSearch: searchSchema,
  beforeLoad: ({ location }) => {
    const { auth } = useAuthStore.getState()
    if (auth.user) return

    const redirectTo = getRelativeLocation(location.href)
    throw redirect({
      to: '/sign-in',
      search: redirectTo
        ? { redirect: sanitizeRedirect(redirectTo) }
        : undefined,
    })
  },
  component: OAuthAuthorize,
})
