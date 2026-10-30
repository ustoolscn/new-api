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
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import type { ChannelFormValues } from '../../../lib/channel-form'

export function UsageAdjustmentFields() {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const enabled = form.watch('usage_adjustment.enabled') ?? false
  const ranges = [
    { label: t('Cache adjustment (%)'), min: 'cache_min', max: 'cache_max' },
    { label: t('Output adjustment (%)'), min: 'output_min', max: 'output_max' },
  ] as const

  return (
    <div className='space-y-3 px-4 py-3'>
      <FormField
        control={form.control}
        name='usage_adjustment.enabled'
        render={({ field }) => (
          <FormItem className='flex items-center justify-between'>
            <FormLabel>{t('Usage adjustment')}</FormLabel>
            <FormControl>
              <Switch
                checked={field.value ?? false}
                onCheckedChange={field.onChange}
              />
            </FormControl>
          </FormItem>
        )}
      />
      <p className='text-muted-foreground text-xs'>
        {t(
          'Sample once per request. Cache changes transfer to ordinary input; total input stays unchanged. Adjusted usage is returned and billed. Text JSON/SSE only.'
        )}
      </p>
      {ranges.map((range) => (
        <div key={range.min} className='space-y-1'>
          <div className='text-xs font-medium'>{range.label}</div>
          <div className='grid grid-cols-2 gap-3'>
            {([range.min, range.max] as const).map((key, index) => (
              <FormField
                key={key}
                control={form.control}
                name={`usage_adjustment.${key}`}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className='text-muted-foreground text-xs'>
                      {index === 0 ? t('Minimum') : t('Maximum')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={-100}
                        max={100}
                        step={0.1}
                        disabled={!enabled}
                        value={field.value ?? 0}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? 0
                              : Number(event.target.value)
                          )
                        }
                      />
                    </FormControl>
                    <div className='h-8 [scrollbar-gutter:stable] overflow-y-auto'>
                      <FormMessage />
                    </div>
                  </FormItem>
                )}
              />
            ))}
          </div>
        </div>
      ))}
      <FormField
        control={form.control}
        name='usage_adjustment'
        render={() => (
          <FormItem>
            <div className='h-8 [scrollbar-gutter:stable] overflow-y-auto'>
              <FormMessage />
            </div>
          </FormItem>
        )}
      />
    </div>
  )
}
