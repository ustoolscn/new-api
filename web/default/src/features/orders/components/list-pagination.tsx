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
import { ArrowLeft01Icon, ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

type ListPaginationProps = {
  page: number
  pageSize: number
  total: number
  disabled?: boolean
  onPageChange: (page: number) => void
}

export function ListPagination(props: ListPaginationProps) {
  const { t } = useTranslation()
  const pageCount = Math.max(1, Math.ceil(props.total / props.pageSize))

  return (
    <div className='flex flex-col items-center justify-between gap-3 border-t px-4 py-3 sm:flex-row'>
      <p className='text-muted-foreground text-sm'>
        {t('Page {{page}} of {{pageCount}}, {{total}} records', {
          page: props.page,
          pageCount,
          total: props.total,
        })}
      </p>
      <div className='flex items-center gap-2'>
        <Button
          type='button'
          variant='outline'
          size='icon-sm'
          disabled={props.page <= 1 || props.disabled}
          onClick={() => props.onPageChange(props.page - 1)}
          aria-label={t('Previous page')}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
        </Button>
        <Button
          type='button'
          variant='outline'
          size='icon-sm'
          disabled={props.page >= pageCount || props.disabled}
          onClick={() => props.onPageChange(props.page + 1)}
          aria-label={t('Next page')}
        >
          <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
        </Button>
      </div>
    </div>
  )
}
