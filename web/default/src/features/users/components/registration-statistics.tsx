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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { BarChart3, RefreshCw, UserPlus } from 'lucide-react'
import { type ReactNode, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import dayjs from '@/lib/dayjs'
import { cn } from '@/lib/utils'

import { getUserRegistrationStatistics } from '../api'
import type { UserRegistrationStatsGranularity } from '../types'

type RangePreset = 7 | 30 | 90

const DATE_FORMAT = 'YYYY-MM-DD'
const RANGE_PRESETS: Array<{ days: RangePreset; labelKey: string }> = [
  { days: 7, labelKey: 'Last 7 Days' },
  { days: 30, labelKey: 'Last 30 Days' },
  { days: 90, labelKey: 'Last 90 Days' },
]
const GRANULARITY_OPTIONS: Array<{
  value: UserRegistrationStatsGranularity
  labelKey: string
}> = [
  { value: 'day', labelKey: 'Daily' },
  { value: 'month', labelKey: 'Monthly' },
  { value: 'year', labelKey: 'Yearly' },
]

function getPresetRange(days: RangePreset) {
  const end = dayjs()
  return {
    start: end.subtract(days - 1, 'day').format(DATE_FORMAT),
    end: end.format(DATE_FORMAT),
  }
}

export function RegistrationStatistics() {
  const { t } = useTranslation()
  const initialRange = getPresetRange(30)
  const [startDate, setStartDate] = useState(initialRange.start)
  const [endDate, setEndDate] = useState(initialRange.end)
  const [activePreset, setActivePreset] = useState<RangePreset | null>(30)
  const [granularity, setGranularity] =
    useState<UserRegistrationStatsGranularity>('day')

  const startTimestamp = dayjs(startDate).startOf('day').unix()
  const endTimestamp = dayjs(endDate).add(1, 'day').startOf('day').unix()
  const rangeValid =
    Number.isFinite(startTimestamp) &&
    Number.isFinite(endTimestamp) &&
    endTimestamp > startTimestamp

  const statisticsQuery = useQuery({
    queryKey: [
      'user-registration-statistics',
      startTimestamp,
      endTimestamp,
      granularity,
    ],
    queryFn: async () => {
      const response = await getUserRegistrationStatistics({
        start_timestamp: startTimestamp,
        end_timestamp: endTimestamp,
        granularity,
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load registration statistics')
        )
      }
      return response.data
    },
    enabled: rangeValid,
    placeholderData: keepPreviousData,
  })

  const chartConfig = useMemo(
    () =>
      ({
        registration_count: {
          label: t('New registrations'),
          color: 'var(--chart-1)',
        },
      }) satisfies ChartConfig,
    [t]
  )

  const applyPreset = (days: RangePreset) => {
    const range = getPresetRange(days)
    setStartDate(range.start)
    setEndDate(range.end)
    setActivePreset(days)
  }

  const items = statisticsQuery.data?.items ?? []
  const totalRegistrations = statisticsQuery.data?.total_registrations ?? 0

  let chartContent: ReactNode
  if (statisticsQuery.isLoading) {
    chartContent = <Skeleton className='h-80 w-full' />
  } else if (statisticsQuery.isError) {
    chartContent = (
      <div className='text-destructive flex h-80 items-center justify-center rounded-lg border border-dashed text-sm'>
        {statisticsQuery.error.message}
      </div>
    )
  } else if (totalRegistrations === 0) {
    chartContent = (
      <div className='text-muted-foreground flex h-80 items-center justify-center rounded-lg border border-dashed text-sm'>
        {t('No registration data')}
      </div>
    )
  } else {
    chartContent = (
      <ChartContainer
        config={chartConfig}
        className='aspect-auto h-80 w-full'
        initialDimension={{ width: 800, height: 320 }}
      >
        <BarChart data={items} margin={{ top: 8, right: 8, left: 8 }}>
          <CartesianGrid vertical={false} />
          <XAxis
            dataKey='bucket_label'
            tickLine={false}
            axisLine={false}
            tickMargin={8}
            minTickGap={16}
          />
          <YAxis
            allowDecimals={false}
            tickLine={false}
            axisLine={false}
            tickMargin={8}
            width={44}
          />
          <ChartTooltip cursor={false} content={<ChartTooltipContent />} />
          <Bar
            dataKey='registration_count'
            fill='var(--color-registration_count)'
            radius={[4, 4, 0, 0]}
          />
        </BarChart>
      </ChartContainer>
    )
  }

  return (
    <div className='space-y-4 pb-4'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Registration Statistics')}</CardTitle>
          <CardDescription>
            {t('Track new user registrations by time period')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='grid gap-4 lg:grid-cols-[180px_180px_1fr_180px_auto] lg:items-end'>
            <div className='space-y-2'>
              <Label htmlFor='registration-stats-start-date'>
                {t('Start Date')}
              </Label>
              <Input
                id='registration-stats-start-date'
                type='date'
                required
                value={startDate}
                onChange={(event) => {
                  setStartDate(event.target.value)
                  setActivePreset(null)
                }}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='registration-stats-end-date'>
                {t('End Date')}
              </Label>
              <Input
                id='registration-stats-end-date'
                type='date'
                required
                value={endDate}
                onChange={(event) => {
                  setEndDate(event.target.value)
                  setActivePreset(null)
                }}
              />
            </div>
            <div className='flex flex-wrap gap-2'>
              {RANGE_PRESETS.map((preset) => (
                <Button
                  key={preset.days}
                  type='button'
                  variant={activePreset === preset.days ? 'default' : 'outline'}
                  onClick={() => applyPreset(preset.days)}
                >
                  {t(preset.labelKey)}
                </Button>
              ))}
            </div>
            <div className='space-y-2'>
              <Label>{t('Granularity')}</Label>
              <Select
                items={GRANULARITY_OPTIONS.map((option) => ({
                  value: option.value,
                  label: t(option.labelKey),
                }))}
                value={granularity}
                onValueChange={(value) =>
                  value &&
                  setGranularity(value as UserRegistrationStatsGranularity)
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder={t('Granularity')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {GRANULARITY_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.labelKey)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <Button
              type='button'
              variant='outline'
              disabled={!rangeValid || statisticsQuery.isFetching}
              onClick={() => statisticsQuery.refetch()}
            >
              <RefreshCw
                className={cn(
                  'size-4',
                  statisticsQuery.isFetching && 'animate-spin'
                )}
              />
              {t('Refresh')}
            </Button>
          </div>
          {!rangeValid ? (
            <p className='text-destructive mt-3 text-sm'>
              {t('End date must not be before start date')}
            </p>
          ) : null}
        </CardContent>
      </Card>

      <div className='grid gap-4 lg:grid-cols-[240px_1fr]'>
        <Card size='sm'>
          <CardContent className='flex min-h-32 flex-col justify-between gap-4'>
            <div className='flex items-center justify-between gap-3'>
              <span className='text-muted-foreground text-sm font-medium'>
                {t('Total registrations')}
              </span>
              <UserPlus className='size-5 text-blue-600' />
            </div>
            <div>
              {statisticsQuery.isLoading ? (
                <Skeleton className='h-9 w-24' />
              ) : (
                <div className='font-mono text-3xl font-semibold tabular-nums'>
                  {totalRegistrations.toLocaleString()}
                </div>
              )}
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('Total registrations in the selected range')}
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className='flex items-center gap-2'>
              <BarChart3 className='text-muted-foreground size-5' />
              <CardTitle>{t('Registration Trend')}</CardTitle>
            </div>
            <CardDescription>
              {t('Registration count by selected granularity')}
            </CardDescription>
          </CardHeader>
          <CardContent>{chartContent}</CardContent>
        </Card>
      </div>
    </div>
  )
}
