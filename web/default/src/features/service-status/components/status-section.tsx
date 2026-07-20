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
import { Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { formatCompactNumber } from '@/lib/format'

import type { ServiceStatusMetric, ServiceStatusPeriod } from '../types'
import { StatusTimeline } from './status-timeline'

const DEFAULT_VISIBLE_ITEMS = 8

type StatusSectionProps = {
  title: string
  description: string
  searchPlaceholder: string
  metrics: ServiceStatusMetric[]
  periods: ServiceStatusPeriod[]
  total: number
  truncated: boolean
}

export function StatusSection(props: StatusSectionProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState(false)
  const normalizedSearch = search.trim().toLowerCase()
  const filteredMetrics = useMemo(() => {
    if (!normalizedSearch) return props.metrics
    return props.metrics.filter((metric) =>
      metric.name.toLowerCase().includes(normalizedSearch)
    )
  }, [normalizedSearch, props.metrics])
  const visibleMetrics =
    expanded || normalizedSearch
      ? filteredMetrics
      : filteredMetrics.slice(0, DEFAULT_VISIBLE_ITEMS)
  const canToggle = !normalizedSearch && filteredMetrics.length > 8

  return (
    <section className='space-y-4'>
      <div className='flex flex-col justify-between gap-3 sm:flex-row sm:items-end'>
        <div>
          <h2 className='text-xl font-semibold tracking-tight'>
            {props.title}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {props.description}
          </p>
        </div>
        {props.metrics.length > DEFAULT_VISIBLE_ITEMS && (
          <div className='relative w-full sm:w-64'>
            <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2' />
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={props.searchPlaceholder}
              className='pl-9'
            />
          </div>
        )}
      </div>

      <div className='bg-card divide-border overflow-hidden rounded-xl border shadow-sm'>
        {visibleMetrics.length === 0 ? (
          <div className='text-muted-foreground px-5 py-10 text-center text-sm'>
            {t('No matching results')}
          </div>
        ) : (
          <div className='divide-y'>
            {visibleMetrics.map((metric) => (
              <div key={metric.name} className='space-y-4 px-4 py-5 sm:px-5'>
                <div className='flex flex-wrap items-start justify-between gap-3'>
                  <div className='min-w-0'>
                    <h3 className='truncate font-medium'>{metric.name}</h3>
                    <p className='text-muted-foreground mt-1 text-xs'>
                      {t('Requests')}:{' '}
                      {formatCompactNumber(metric.request_count)}
                    </p>
                  </div>
                  <div className='text-right'>
                    <div className='font-mono text-sm font-semibold tabular-nums'>
                      {metric.success_rate == null
                        ? t('No data')
                        : `${metric.success_rate.toFixed(2)}%`}
                    </div>
                    <div className='text-muted-foreground mt-1 text-xs'>
                      {t('Success rate')}
                    </div>
                  </div>
                </div>
                <StatusTimeline
                  periods={props.periods}
                  series={metric.series}
                />
              </div>
            ))}
          </div>
        )}
      </div>

      <div className='flex flex-wrap items-center justify-between gap-3'>
        <p className='text-muted-foreground text-xs'>
          {props.truncated
            ? t('Showing {{visible}} of {{total}} active items.', {
                visible: props.metrics.length,
                total: props.total,
              })
            : t('{{count}} active items', { count: props.total })}
        </p>
        {canToggle && (
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={() => setExpanded((value) => !value)}
          >
            {expanded ? t('Collapse') : t('Show all')}
          </Button>
        )}
      </div>
    </section>
  )
}
