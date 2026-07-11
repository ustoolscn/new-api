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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  getClientIPBlacklistSettings,
  updateClientIPBlacklistSettings,
} from '../api'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import type { UpdateClientIPBlacklistRequest } from '../types'
import { splitIPRules } from './client-ip-blacklist-utils'

const clientIPBlacklistSchema = z.object({
  blacklist_enabled: z.boolean(),
  blacklist: z.string(),
  trusted_proxies: z.string(),
})

type ClientIPBlacklistFormValues = z.infer<typeof clientIPBlacklistSchema>

type ClientIPBlacklistSectionProps = {
  defaultValues: {
    'client_ip_setting.blacklist_enabled': boolean
    'client_ip_setting.blacklist': string[]
    'client_ip_setting.trusted_proxies': string[]
  }
}

function buildFormValues(
  enabled: boolean,
  blacklist: string[],
  trustedProxies: string[]
): ClientIPBlacklistFormValues {
  return {
    blacklist_enabled: enabled,
    blacklist: blacklist.join('\n'),
    trusted_proxies: trustedProxies.join('\n'),
  }
}

function normalizeFormValues(
  values: ClientIPBlacklistFormValues,
  confirmSelfBlock: boolean
): UpdateClientIPBlacklistRequest {
  return {
    blacklist_enabled: values.blacklist_enabled,
    blacklist: splitIPRules(values.blacklist),
    trusted_proxies: splitIPRules(values.trusted_proxies),
    confirm_self_block: confirmSelfBlock,
  }
}

export function ClientIPBlacklistSection(props: ClientIPBlacklistSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const pendingPayloadRef = useRef<UpdateClientIPBlacklistRequest | null>(null)
  const baselineRef = useRef(
    buildFormValues(
      props.defaultValues['client_ip_setting.blacklist_enabled'],
      props.defaultValues['client_ip_setting.blacklist'],
      props.defaultValues['client_ip_setting.trusted_proxies']
    )
  )

  const form = useForm<ClientIPBlacklistFormValues>({
    resolver: zodResolver(clientIPBlacklistSchema),
    defaultValues: baselineRef.current,
  })

  const settingsQuery = useQuery({
    queryKey: ['client-ip-blacklist-settings'],
    queryFn: getClientIPBlacklistSettings,
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    if (!settingsQuery.data?.success) return
    const nextValues = buildFormValues(
      settingsQuery.data.data.blacklist_enabled,
      settingsQuery.data.data.blacklist,
      settingsQuery.data.data.trusted_proxies
    )
    baselineRef.current = nextValues
    form.reset(nextValues)
  }, [form, settingsQuery.data])

  const updateMutation = useMutation({
    mutationFn: updateClientIPBlacklistSettings,
    onSuccess: (response) => {
      const nextValues = buildFormValues(
        response.data.blacklist_enabled,
        response.data.blacklist,
        response.data.trusted_proxies
      )
      baselineRef.current = nextValues
      form.reset(nextValues)
      queryClient.setQueryData(['client-ip-blacklist-settings'], response)
      pendingPayloadRef.current = null
      setConfirmOpen(false)
      toast.success(t('Client IP blacklist settings saved'))
    },
    onError: (error: unknown, variables) => {
      if (
        axios.isAxiosError(error) &&
        error.response?.status === 409 &&
        error.response.data?.code ===
          'client_ip_self_block_confirmation_required'
      ) {
        pendingPayloadRef.current = variables
        setConfirmOpen(true)
        return
      }
      let message = t('Request failed')
      if (axios.isAxiosError(error)) {
        message = error.response?.data?.message || error.message
      } else if (error instanceof Error) {
        message = error.message
      }
      toast.error(message)
    },
  })

  const submit = (values: ClientIPBlacklistFormValues) => {
    updateMutation.mutate(normalizeFormValues(values, false))
  }

  const currentIP = settingsQuery.data?.data.current_ip

  return (
    <SettingsSection title={t('Client IP Blacklist')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(submit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(submit)}
            isSaving={updateMutation.isPending}
            saveLabel='Save client IP blacklist'
          />

          <Alert>
            <AlertTitle>{t('Current recognized client IP')}</AlertTitle>
            <AlertDescription>
              {currentIP ?? t('Loading current client IP...')}
            </AlertDescription>
          </Alert>

          <Alert variant='destructive'>
            <AlertTitle>
              {t('This blacklist blocks the entire site')}
            </AlertTitle>
            <AlertDescription>
              {t(
                'Matching IPs cannot open the website, sign in, use admin APIs, or call model APIs. Only /api/status remains available.'
              )}
            </AlertDescription>
          </Alert>

          <FormField
            control={form.control}
            name='blacklist_enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable client IP blacklist')}</FormLabel>
                  <FormDescription>
                    {t('Reject matching client IPs before authentication')}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='blacklist'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Blocked client IPs')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={6}
                    placeholder={'203.0.113.7\n203.0.113.0/24\n2001:db8::/48'}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Enter one IPv4, IPv6, or CIDR rule per line')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='trusted_proxies'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Trusted proxy IPs')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    placeholder={'127.0.0.1\n10.0.0.0/8'}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Forwarded IP headers are used only when the direct connection comes from one of these proxies. Leave empty for direct deployments.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('Block your current IP?')}
        desc={t(
          'The new blacklist matches your current client IP. After saving, this browser will lose access and you must recover from another IP or edit the server options directly.'
        )}
        confirmText={t('Save and block my IP')}
        destructive
        isLoading={updateMutation.isPending}
        handleConfirm={() => {
          const payload = pendingPayloadRef.current
          if (!payload) return
          updateMutation.mutate({ ...payload, confirm_self_block: true })
        }}
      />
    </SettingsSection>
  )
}
