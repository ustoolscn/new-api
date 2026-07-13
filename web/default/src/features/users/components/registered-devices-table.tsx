import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  ColumnDef,
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
} from '@tanstack/react-table'
import dayjs from 'dayjs'
import { Ban, Network, ScanSearch, ShieldCheck } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'

import { getRegisteredDevices, setRegisteredDeviceBanned } from '../api'
import type { RegisteredDevice } from '../types'

function formatTimestamp(timestamp: number): string {
  if (!timestamp) return '-'
  return dayjs.unix(timestamp).format('YYYY-MM-DD HH:mm:ss')
}

function getDeviceRowClassName(banned: boolean, isMobile: boolean) {
  if (!banned) return undefined
  return isMobile ? DISABLED_ROW_MOBILE : DISABLED_ROW_DESKTOP
}

export function RegisteredDevicesTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [globalFilter, setGlobalFilter] = useState('')
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })
  const [selectedDevice, setSelectedDevice] = useState<RegisteredDevice | null>(
    null
  )

  const status =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | 'true'
      | 'false'
      | undefined) ?? ''
  const duplicate =
    (columnFilters.find((filter) => filter.id === 'duplicate')?.value as
      | 'device'
      | 'ip'
      | undefined) ?? ''

  const handleGlobalFilterChange = useCallback<OnChangeFn<string>>((value) => {
    setGlobalFilter(value)
    setPagination((current) => ({ ...current, pageIndex: 0 }))
  }, [])

  const handleColumnFiltersChange = useCallback<OnChangeFn<ColumnFiltersState>>(
    (value) => {
      setColumnFilters(value)
      setPagination((current) => ({ ...current, pageIndex: 0 }))
    },
    []
  )

  const devicesQuery = useQuery({
    queryKey: [
      'registered-devices',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      status,
      duplicate,
    ],
    queryFn: async () => {
      const response = await getRegisteredDevices({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        keyword: globalFilter.trim(),
        status,
        duplicate,
      })
      if (!response.success) {
        throw new Error(response.message || t('Failed to load devices'))
      }
      return {
        items: response.data?.items ?? [],
        total: response.data?.total ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const statusMutation = useMutation({
    mutationFn: async (device: RegisteredDevice) => {
      const response = await setRegisteredDeviceBanned(
        device.id,
        !device.banned
      )
      if (!response.success) {
        throw new Error(response.message || t('Failed to update device status'))
      }
    },
    onSuccess: () => {
      toast.success(t('Device status updated'))
      setSelectedDevice(null)
      queryClient.invalidateQueries({ queryKey: ['registered-devices'] })
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to update device status'))
    },
  })

  const columns = useMemo<ColumnDef<RegisteredDevice>[]>(
    () => [
      {
        accessorKey: 'username',
        header: t('User'),
        cell: ({ row }) => (
          <div className='min-w-32'>
            <div className='font-medium'>{row.original.username || '-'}</div>
            <div className='text-muted-foreground text-xs'>
              ID: {row.original.user_id}
            </div>
          </div>
        ),
      },
      {
        accessorKey: 'fingerprint_hash',
        header: t('Device fingerprint'),
        cell: ({ row }) => (
          <div className='flex max-w-56 flex-col items-start gap-1'>
            <StatusBadge
              label={row.original.fingerprint_hash}
              variant='neutral'
              className='max-w-full font-mono'
            />
            {row.original.device_user_count > 1 && (
              <StatusBadge
                label={t('{{count}} users share this device', {
                  count: row.original.device_user_count,
                })}
                icon={ScanSearch}
                variant='warning'
                copyable={false}
              />
            )}
          </div>
        ),
      },
      {
        accessorKey: 'first_ip',
        header: t('Registration IP'),
        cell: ({ row }) => (
          <div className='flex flex-col items-start gap-1'>
            <span className='font-mono text-xs'>
              {row.original.first_ip || '-'}
            </span>
            {row.original.ip_user_count > 1 && (
              <StatusBadge
                label={t('{{count}} users share this IP', {
                  count: row.original.ip_user_count,
                })}
                icon={Network}
                variant='warning'
                copyable={false}
              />
            )}
          </div>
        ),
      },
      {
        accessorKey: 'contact',
        header: t('Contact'),
        cell: ({ row }) => (
          <div className='max-w-52 text-xs'>
            <div className='truncate'>{row.original.email || '-'}</div>
            {row.original.phone && (
              <div className='text-muted-foreground truncate'>
                {row.original.phone}
              </div>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'user_agent',
        header: t('User Agent'),
        cell: ({ row }) => (
          <span
            className='block max-w-64 truncate text-xs'
            title={row.original.user_agent}
          >
            {row.original.user_agent || '-'}
          </span>
        ),
      },
      {
        accessorKey: 'first_seen_at',
        header: t('Registered at'),
        cell: ({ row }) => (
          <span className='text-xs whitespace-nowrap'>
            {formatTimestamp(row.original.first_seen_at)}
          </span>
        ),
      },
      {
        id: 'status',
        accessorFn: (device) => (device.banned ? 'banned' : 'active'),
        header: t('Status'),
        cell: ({ row }) => (
          <StatusBadge
            label={row.original.banned ? t('Banned') : t('Active')}
            variant={row.original.banned ? 'danger' : 'success'}
            copyable={false}
          />
        ),
      },
      {
        id: 'duplicate',
        accessorFn: () => '',
        enableHiding: true,
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <Button
            size='sm'
            variant={row.original.banned ? 'outline' : 'destructive'}
            onClick={() => setSelectedDevice(row.original)}
          >
            {row.original.banned ? (
              <ShieldCheck size={15} />
            ) : (
              <Ban size={15} />
            )}
            {row.original.banned ? t('Unban') : t('Ban')}
          </Button>
        ),
        enableSorting: false,
      },
    ],
    [t]
  )

  const { table } = useDataTable({
    data: devicesQuery.data?.items ?? [],
    columns,
    columnFilters,
    globalFilter,
    pagination,
    onPaginationChange: setPagination,
    onGlobalFilterChange: handleGlobalFilterChange,
    onColumnFiltersChange: handleColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    initialColumnVisibility: { duplicate: false },
    totalCount: devicesQuery.data?.total ?? 0,
  })

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={devicesQuery.isLoading}
        isFetching={devicesQuery.isFetching}
        emptyTitle={t('No registered devices found')}
        emptyDescription={t(
          'Devices are recorded when users register from the default frontend.'
        )}
        skeletonKeyPrefix='registered-devices-skeleton'
        applyHeaderSize
        toolbarProps={{
          searchPlaceholder: t(
            'Search fingerprint, user, email, phone or IP...'
          ),
          filters: [
            {
              columnId: 'status',
              title: t('Status'),
              singleSelect: true,
              options: [
                { label: t('Active'), value: 'false' },
                { label: t('Banned'), value: 'true' },
              ],
            },
            {
              columnId: 'duplicate',
              title: t('Shared identifiers'),
              singleSelect: true,
              options: [
                { label: t('Shared device'), value: 'device' },
                { label: t('Shared IP'), value: 'ip' },
              ],
            },
          ],
        }}
        getRowClassName={(row, { isMobile }) =>
          getDeviceRowClassName(row.original.banned, isMobile)
        }
      />

      <ConfirmDialog
        open={selectedDevice !== null}
        onOpenChange={(open) => !open && setSelectedDevice(null)}
        title={
          selectedDevice?.banned
            ? t('Unban this device?')
            : t('Ban this device?')
        }
        desc={
          selectedDevice?.banned
            ? t(
                'The device can register and sign in again. Disabled users and API keys are not restored automatically.'
              )
            : t(
                'All users and API keys linked to this device will be disabled. The device will be blocked from registration, sign-in and authenticated API access.'
              )
        }
        confirmText={selectedDevice?.banned ? t('Unban') : t('Ban device')}
        destructive={!selectedDevice?.banned}
        isLoading={statusMutation.isPending}
        handleConfirm={() => {
          if (selectedDevice) statusMutation.mutate(selectedDevice)
        }}
      />
    </>
  )
}
