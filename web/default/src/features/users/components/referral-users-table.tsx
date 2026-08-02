import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type {
  ColumnDef,
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
} from '@tanstack/react-table'
import { Eye } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { formatQuota } from '@/lib/format'

import { getReferralInviters } from '../api'
import type { ReferralInviterSummary } from '../types'
import { UserReferralDrawer } from './user-referral-drawer'

export function ReferralUsersTable() {
  const { t } = useTranslation()
  const [globalFilter, setGlobalFilter] = useState('')
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })
  const [selectedUser, setSelectedUser] =
    useState<ReferralInviterSummary | null>(null)

  const handleGlobalFilterChange = useCallback<OnChangeFn<string>>((value) => {
    setGlobalFilter(value)
    setPagination((current) => ({ ...current, pageIndex: 0 }))
  }, [])

  const referralsQuery = useQuery({
    queryKey: [
      'admin-referral-users',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
    ],
    queryFn: async () => {
      const response = await getReferralInviters({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        keyword: globalFilter,
      })
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to load referral information')
        )
      }
      return {
        items: response.data?.items ?? [],
        total: response.data?.total ?? 0,
      }
    },
    placeholderData: keepPreviousData,
  })

  const columns = useMemo<ColumnDef<ReferralInviterSummary>[]>(
    () => [
      {
        accessorKey: 'username',
        header: t('User'),
        cell: ({ row }) => (
          <Button
            type='button'
            variant='link'
            className='h-auto min-w-36 justify-start p-0 text-left'
            onClick={() => setSelectedUser(row.original)}
          >
            <span>
              <span className='block font-medium'>
                {row.original.display_name || row.original.username}
              </span>
              <span className='text-muted-foreground block text-xs'>
                @{row.original.username} · ID: {row.original.id}
              </span>
            </span>
          </Button>
        ),
      },
      {
        accessorKey: 'invite_count',
        header: t('Invited users'),
        cell: ({ row }) => (
          <span className='tabular-nums'>{row.original.invite_count}</span>
        ),
      },
      {
        accessorKey: 'rewarded_invite_count',
        header: t('Rewarded invites'),
        cell: ({ row }) => (
          <span className='tabular-nums'>
            {row.original.rewarded_invite_count}
          </span>
        ),
      },
      {
        accessorKey: 'invite_reward_total_quota',
        header: t('Registration rewards'),
        cell: ({ row }) => (
          <div className='min-w-28 text-left'>
            <div className='font-medium tabular-nums'>
              {formatQuota(row.original.invite_reward_total_quota)}
            </div>
            <div className='text-muted-foreground text-xs tabular-nums'>
              {t('Pending rewards')}:{' '}
              {formatQuota(row.original.invite_reward_pending_quota)}
            </div>
          </div>
        ),
      },
      {
        accessorKey: 'invitee_consume_total',
        header: t('Invitee consumption'),
        cell: ({ row }) => (
          <span className='font-medium tabular-nums'>
            {formatQuota(row.original.invitee_consume_total)}
          </span>
        ),
      },
      {
        accessorKey: 'total_quota',
        header: t('Total commission'),
        cell: ({ row }) => (
          <div className='min-w-28 text-left'>
            <div className='font-medium tabular-nums'>
              {formatQuota(row.original.total_quota)}
            </div>
            <div className='text-muted-foreground text-xs tabular-nums'>
              {t('Pending commission')}:{' '}
              {formatQuota(row.original.pending_quota)}
            </div>
          </div>
        ),
      },
      {
        accessorKey: 'claimed_quota',
        header: t('Claimed'),
        cell: ({ row }) => (
          <span className='font-medium tabular-nums'>
            {formatQuota(row.original.claimed_quota)}
          </span>
        ),
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => setSelectedUser(row.original)}
          >
            <Eye data-icon='inline-start' />
            {t('View details')}
          </Button>
        ),
        enableSorting: false,
      },
    ],
    [t]
  )

  const { table } = useDataTable({
    data: referralsQuery.data?.items ?? [],
    columns,
    columnFilters,
    globalFilter,
    pagination,
    onPaginationChange: setPagination,
    onColumnFiltersChange: setColumnFilters,
    onGlobalFilterChange: handleGlobalFilterChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: referralsQuery.data?.total ?? 0,
  })

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={referralsQuery.isLoading}
        isFetching={referralsQuery.isFetching}
        emptyTitle={
          referralsQuery.isError
            ? t('Failed to load referral information')
            : t('No referral records found')
        }
        emptyDescription={
          referralsQuery.isError
            ? referralsQuery.error.message
            : t('Users with invitations or referral rewards will appear here.')
        }
        skeletonKeyPrefix='referral-users-skeleton'
        applyHeaderSize
        toolbarProps={{
          searchPlaceholder: t('Search referral users...'),
        }}
      />

      <UserReferralDrawer
        user={selectedUser}
        onOpenChange={(open) => !open && setSelectedUser(null)}
      />
    </>
  )
}
