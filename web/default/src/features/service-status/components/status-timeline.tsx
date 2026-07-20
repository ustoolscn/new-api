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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { ServiceStatusPeriod, ServiceStatusPoint } from '../types'

type StatusTimelineProps = {
  periods: ServiceStatusPeriod[]
  series: ServiceStatusPoint[]
  className?: string
}

function getPointClass(point: ServiceStatusPoint): string {
  if (point.success_rate == null || point.request_count === 0) {
    return 'bg-muted-foreground/20'
  }
  if (point.success_rate >= 99) {
    return 'bg-emerald-500'
  }
  if (point.success_rate >= 95) {
    return 'bg-amber-500'
  }
  return 'bg-rose-500'
}

export function StatusTimeline(props: StatusTimelineProps) {
  const { t } = useTranslation()
  const endLabel = props.periods.at(-1)?.bucket_label ?? ''

  return (
    <div className={cn('space-y-1.5', props.className)}>
      <div className='overflow-x-auto pb-1'>
        <div
          className='flex h-7 min-w-[680px] items-stretch gap-1'
          role='img'
          aria-label={t('Request success timeline')}
        >
          {props.series.map((point, index) => {
            const period = props.periods[index]
            const rateLabel =
              point.success_rate == null
                ? t('No data')
                : `${point.success_rate.toFixed(2)}%`
            const detail = `${period?.bucket_label ?? ''} · ${t('Requests')}: ${point.request_count.toLocaleString()} · ${t('Success rate')}: ${rateLabel}`
            return (
              <div
                key={period?.bucket_start ?? index}
                className={cn(
                  'min-w-1 flex-1 rounded-[3px] transition-opacity hover:opacity-75',
                  getPointClass(point)
                )}
                title={detail}
                aria-label={detail}
              />
            )
          })}
        </div>
      </div>
      <div className='text-muted-foreground flex justify-between gap-4 text-[11px] tabular-nums'>
        <span>{props.periods[0]?.bucket_label}</span>
        <span>{endLabel}</span>
      </div>
    </div>
  )
}
