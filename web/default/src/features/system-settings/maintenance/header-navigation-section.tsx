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
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import {
  SettingsControlChildren,
  SettingsForm,
  SettingsSwitchContent,
  SettingsControlGroup,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  HEADER_NAV_DEFAULT,
  type HeaderNavModulesConfig,
  serializeHeaderNavModules,
} from './config'

const headerNavSchema = z.object({
  home: z.boolean(),
  console: z.boolean(),
  pricingEnabled: z.boolean(),
  pricingRequireAuth: z.boolean(),
  rankingsEnabled: z.boolean(),
  rankingsRequireAuth: z.boolean(),
  rankingsDisplayMultiplier: z.number().min(0),
  rankingsDisplayJitterRatio: z.number().min(0),
  docs: z.boolean(),
  serviceStatus: z.boolean(),
  about: z.boolean(),
})

type HeaderNavFormValues = z.infer<typeof headerNavSchema>
type HeaderNavBooleanKey = Exclude<
  keyof HeaderNavFormValues,
  'rankingsDisplayMultiplier' | 'rankingsDisplayJitterRatio'
>

type HeaderNavigationSectionProps = {
  config: HeaderNavModulesConfig
  initialSerialized: string
  rankingsDisplayMultiplier: string
  rankingsDisplayJitterRatio: string
}

const parseDisplayNumber = (value: string | undefined, fallback: number) => {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback
}

const toOptionNumber = (value: number) => {
  if (!Number.isFinite(value) || value < 0) {
    return '0'
  }
  return String(value)
}

const toFormValues = (
  config: HeaderNavModulesConfig,
  multiplier: string,
  jitterRatio: string
): HeaderNavFormValues => ({
  home:
    config.home === undefined ? HEADER_NAV_DEFAULT.home : Boolean(config.home),
  console:
    config.console === undefined
      ? HEADER_NAV_DEFAULT.console
      : Boolean(config.console),
  pricingEnabled:
    config.pricing?.enabled === undefined
      ? HEADER_NAV_DEFAULT.pricing.enabled
      : Boolean(config.pricing.enabled),
  pricingRequireAuth:
    config.pricing?.requireAuth === undefined
      ? HEADER_NAV_DEFAULT.pricing.requireAuth
      : Boolean(config.pricing.requireAuth),
  rankingsEnabled:
    config.rankings?.enabled === undefined
      ? HEADER_NAV_DEFAULT.rankings.enabled
      : Boolean(config.rankings.enabled),
  rankingsRequireAuth:
    config.rankings?.requireAuth === undefined
      ? HEADER_NAV_DEFAULT.rankings.requireAuth
      : Boolean(config.rankings.requireAuth),
  rankingsDisplayMultiplier: parseDisplayNumber(multiplier, 1),
  rankingsDisplayJitterRatio: parseDisplayNumber(jitterRatio, 0),
  docs:
    config.docs === undefined ? HEADER_NAV_DEFAULT.docs : Boolean(config.docs),
  serviceStatus:
    config.serviceStatus === undefined
      ? HEADER_NAV_DEFAULT.serviceStatus
      : Boolean(config.serviceStatus),
  about:
    config.about === undefined
      ? HEADER_NAV_DEFAULT.about
      : Boolean(config.about),
})

export function HeaderNavigationSection({
  config,
  initialSerialized,
  rankingsDisplayMultiplier,
  rankingsDisplayJitterRatio,
}: HeaderNavigationSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(
    () =>
      toFormValues(
        config,
        rankingsDisplayMultiplier,
        rankingsDisplayJitterRatio
      ),
    [config, rankingsDisplayJitterRatio, rankingsDisplayMultiplier]
  )

  const form = useForm<HeaderNavFormValues>({
    resolver: zodResolver(headerNavSchema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    form.reset(formDefaults)
  }, [formDefaults, form])

  const onSubmit = async (values: HeaderNavFormValues) => {
    const payload: HeaderNavModulesConfig = {
      ...config,
      home: values.home,
      console: values.console,
      docs: values.docs,
      serviceStatus: values.serviceStatus,
      about: values.about,
      pricing: {
        ...(config.pricing ?? HEADER_NAV_DEFAULT.pricing),
        enabled: values.pricingEnabled,
        requireAuth: values.pricingRequireAuth,
      },
      rankings: {
        ...(config.rankings ?? HEADER_NAV_DEFAULT.rankings),
        enabled: values.rankingsEnabled,
        requireAuth: values.rankingsRequireAuth,
      },
    }

    const serialized = serializeHeaderNavModules(payload)
    const updates: Array<{ key: string; value: string }> = []
    if (serialized !== initialSerialized) {
      updates.push({
        key: 'HeaderNavModules',
        value: serialized,
      })
    }

    const nextMultiplier = toOptionNumber(values.rankingsDisplayMultiplier)
    if (
      nextMultiplier !==
      toOptionNumber(parseDisplayNumber(rankingsDisplayMultiplier, 1))
    ) {
      updates.push({
        key: 'RankingsDisplayMultiplier',
        value: nextMultiplier,
      })
    }

    const nextJitterRatio = toOptionNumber(values.rankingsDisplayJitterRatio)
    if (
      nextJitterRatio !==
      toOptionNumber(parseDisplayNumber(rankingsDisplayJitterRatio, 0))
    ) {
      updates.push({
        key: 'RankingsDisplayJitterRatio',
        value: nextJitterRatio,
      })
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const resetToDefault = () => {
    form.reset(toFormValues(HEADER_NAV_DEFAULT, '1', '0'))
  }

  const simpleModules: Array<{
    key: HeaderNavBooleanKey
    title: string
    description: string
  }> = [
    {
      key: 'home',
      title: t('Home'),
      description: t('Landing page with system overview.'),
    },
    {
      key: 'console',
      title: t('Console'),
      description: t('User dashboard and quota controls.'),
    },
    {
      key: 'docs',
      title: t('Docs'),
      description: t('Documentation or external knowledge base.'),
    },
    {
      key: 'serviceStatus',
      title: t('Service Status'),
      description: t('Public service health and performance page.'),
    },
    {
      key: 'about',
      title: t('About'),
      description: t('Static page describing the platform.'),
    },
  ]

  const accessModules: Array<{
    enabledKey: HeaderNavBooleanKey
    requireAuthKey: HeaderNavBooleanKey
    requireAuthDependsOn: 'pricingEnabled' | 'rankingsEnabled'
    title: string
    description: string
    requireAuthTitle: string
    requireAuthDescription: string
  }> = [
    {
      enabledKey: 'pricingEnabled',
      requireAuthKey: 'pricingRequireAuth',
      requireAuthDependsOn: 'pricingEnabled',
      title: t('Model Square'),
      description: t('Public model catalog and pricing page.'),
      requireAuthTitle: t('Require login to view models'),
      requireAuthDescription: t(
        'Visitors must authenticate before accessing the pricing directory.'
      ),
    },
    {
      enabledKey: 'rankingsEnabled',
      requireAuthKey: 'rankingsRequireAuth',
      requireAuthDependsOn: 'rankingsEnabled',
      title: t('Rankings'),
      description: t('Public rankings page based on live usage data.'),
      requireAuthTitle: t('Require login to view rankings'),
      requireAuthDescription: t(
        'Visitors must authenticate before accessing the rankings page.'
      ),
    },
  ]

  return (
    <SettingsSection title={t('Header navigation')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            onReset={resetToDefault}
            isSaving={updateOption.isPending}
            resetLabel='Reset to default'
            saveLabel='Save navigation'
          />
          <div className='grid gap-4 md:grid-cols-2'>
            {simpleModules.map((module) => (
              <FormField
                key={module.key}
                control={form.control}
                name={module.key}
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{module.title}</FormLabel>
                      <FormDescription>{module.description}</FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </SettingsSwitchItem>
                )}
              />
            ))}
          </div>

          <div className='grid gap-4 lg:grid-cols-2'>
            {accessModules.map((module) => (
              <SettingsControlGroup key={module.enabledKey}>
                <FormField
                  control={form.control}
                  name={module.enabledKey}
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{module.title}</FormLabel>
                        <FormDescription>{module.description}</FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormMessage />
                    </SettingsSwitchItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name={module.requireAuthKey}
                  render={({ field }) => (
                    <SettingsControlChildren>
                      <SettingsSwitchItem className='py-2'>
                        <SettingsSwitchContent>
                          <FormLabel>{module.requireAuthTitle}</FormLabel>
                          <FormDescription>
                            {module.requireAuthDescription}
                          </FormDescription>
                        </SettingsSwitchContent>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            onCheckedChange={field.onChange}
                            disabled={!form.watch(module.requireAuthDependsOn)}
                          />
                        </FormControl>
                        <FormMessage />
                      </SettingsSwitchItem>
                    </SettingsControlChildren>
                  )}
                />
              </SettingsControlGroup>
            ))}
          </div>
          <div className='space-y-4 rounded-lg border p-4'>
            <div>
              <h3 className='text-base font-medium'>
                {t('Public analytics display values')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Applies to request volumes shown on rankings and service status. Raw usage logs and billing stay unchanged.'
                )}
              </p>
            </div>
            <div className='grid gap-4 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='rankingsDisplayMultiplier'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Display multiplier')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min='0'
                        step='0.01'
                        placeholder='1'
                        value={field.value}
                        onBlur={field.onBlur}
                        name={field.name}
                        ref={field.ref}
                        onChange={(event) =>
                          field.onChange(event.currentTarget.valueAsNumber)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Displayed value equals raw value multiplied by this number.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='rankingsDisplayJitterRatio'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Random jitter ratio')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min='0'
                        step='0.01'
                        placeholder='0'
                        value={field.value}
                        onBlur={field.onBlur}
                        name={field.name}
                        ref={field.ref}
                        onChange={(event) =>
                          field.onChange(event.currentTarget.valueAsNumber)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Adds stable positive random noise. Example: 0.05 means up to 5%.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
