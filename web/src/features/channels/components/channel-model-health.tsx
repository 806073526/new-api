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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, RefreshCw, RotateCcw } from 'lucide-react'
import { useDeferredValue, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatTimestamp } from '@/lib/format'

import {
  getChannelHealthSummary,
  getChannelModelHealth,
  resetChannelModelHealth,
} from '../api'
import {
  getChannelModelHealthClosedDisplayState,
  getChannelModelHealthDisplayState,
  type ChannelModelHealthDisplayState,
} from '../lib/channel-model-health'
import type { ChannelModelHealth } from '../types'
import { ChannelModelHealthLastRequest } from './channel-model-health-last-request'

const PAGE_SIZE = 30

const stateOptions = [
  { value: 'all', label: 'All states' },
  { value: 'open', label: 'Circuit open' },
  { value: 'half_open', label: 'Probing' },
  { value: 'suspect', label: 'Suspect' },
  { value: 'closed', label: 'Healthy' },
]

function HealthStateBadge({
  state,
  healthRecordExists,
}: {
  state: ChannelModelHealthDisplayState
  healthRecordExists?: boolean
}) {
  const { t } = useTranslation()
  if (state === 'open') {
    return <Badge variant='destructive'>{t('Circuit open')}</Badge>
  }
  if (state === 'ready') {
    return (
      <Badge
        variant='outline'
        className='border-sky-500/50 text-sky-700 dark:text-sky-300'
      >
        {t('Waiting for probe')}
      </Badge>
    )
  }
  if (state === 'half_open') {
    return <Badge variant='secondary'>{t('Probing')}</Badge>
  }
  if (state === 'suspect') {
    return (
      <Badge
        variant='outline'
        className='border-amber-500/50 text-amber-700 dark:text-amber-300'
      >
        {t('Suspect')}
      </Badge>
    )
  }
  const closedState = getChannelModelHealthClosedDisplayState({
    health_record_exists: healthRecordExists,
  })
  return (
    <Badge variant='outline'>
      {t(closedState === 'healthy' ? 'Healthy' : 'Recovered')}
    </Badge>
  )
}

function formatHealthTime(value: number) {
  return value > 0 ? formatTimestamp(value) : '-'
}

export function ChannelModelHealthTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [state, setState] = useState('all')
  const [modelFilter, setModelFilter] = useState('')
  const deferredModelFilter = useDeferredValue(modelFilter.trim())

  const healthQuery = useQuery({
    queryKey: ['channel-model-health', page, state, deferredModelFilter],
    queryFn: () =>
      getChannelModelHealth({
        p: page,
        page_size: PAGE_SIZE,
        state: state === 'all' ? undefined : state,
        model: deferredModelFilter || undefined,
      }),
    placeholderData: (previousData) => previousData,
    refetchInterval: 15_000,
  })

  const summaryQuery = useQuery({
    queryKey: ['channel-model-health-summary'],
    queryFn: getChannelHealthSummary,
    refetchInterval: 15_000,
  })

  const totals = useMemo(
    () =>
      (summaryQuery.data?.data ?? []).reduce(
        (result, item) => ({
          open: result.open + item.open,
          ready: result.ready + (item.ready ?? 0),
          halfOpen: result.halfOpen + item.half_open,
          suspect: result.suspect + item.suspect,
          healthy: result.healthy + (item.healthy ?? 0),
          recovered: result.recovered + item.closed,
        }),
        {
          open: 0,
          ready: 0,
          halfOpen: 0,
          suspect: 0,
          healthy: 0,
          recovered: 0,
        }
      ),
    [summaryQuery.data]
  )

  const resetMutation = useMutation({
    mutationFn: (item: ChannelModelHealth) =>
      resetChannelModelHealth({
        channel_id: item.channel_id,
        group: item.group,
        model: item.model,
      }),
    onSuccess: async (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to reset model health'))
        return
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['channel-model-health'] }),
        queryClient.invalidateQueries({
          queryKey: ['channel-model-health-summary'],
        }),
      ])
      toast.success(t('Model health reset'))
    },
  })

  const items = healthQuery.data?.data?.items ?? []
  const total = healthQuery.data?.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))
  let tableRows = (
    <>
      {items.map((item) => (
        <TableRow key={`${item.channel_id}:${item.group}:${item.model}`}>
          <TableCell>
            <div className='flex flex-col'>
              <span className='font-medium'>{item.channel_name}</span>
              <span className='text-muted-foreground text-xs'>
                #{item.channel_id}
              </span>
            </div>
          </TableCell>
          <TableCell>{item.group}</TableCell>
          <TableCell className='max-w-64 truncate font-mono'>
            {item.model}
          </TableCell>
          <TableCell>
            <HealthStateBadge
              state={getChannelModelHealthDisplayState(
                item,
                Math.floor(Date.now() / 1000)
              )}
              healthRecordExists={item.health_record_exists}
            />
          </TableCell>
          <TableCell>{item.failure_count}</TableCell>
          <TableCell>{formatHealthTime(item.last_failure_at)}</TableCell>
          <TableCell>{formatHealthTime(item.cooldown_until)}</TableCell>
          <TableCell className='max-w-80'>
            <span className='block truncate' title={item.last_error}>
              {item.last_status_code > 0
                ? `${item.last_status_code} · ${item.last_error_code || item.last_error}`
                : item.last_error || '-'}
            </span>
          </TableCell>
          <TableCell className='min-w-52'>
            <ChannelModelHealthLastRequest item={item} />
          </TableCell>
          <TableCell className='text-right'>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    aria-label={t('Reset model health')}
                    disabled={resetMutation.isPending}
                    onClick={() => resetMutation.mutate(item)}
                  />
                }
              >
                <RotateCcw />
              </TooltipTrigger>
              <TooltipContent>{t('Reset model health')}</TooltipContent>
            </Tooltip>
          </TableCell>
        </TableRow>
      ))}
    </>
  )
  if (healthQuery.isLoading) {
    tableRows = (
      <TableRow>
        <TableCell colSpan={10} className='h-28 text-center'>
          {t('Loading...')}
        </TableCell>
      </TableRow>
    )
  } else if (items.length === 0) {
    tableRows = (
      <TableRow>
        <TableCell
          colSpan={10}
          className='text-muted-foreground h-28 text-center'
        >
          {t('No model health records')}
        </TableCell>
      </TableRow>
    )
  }

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      <div className='flex flex-col gap-3 border-b pb-3 lg:flex-row lg:items-center lg:justify-between'>
        <div className='flex min-w-0 flex-wrap items-center gap-2'>
          <Badge variant='destructive'>
            {t('Circuit open')} {totals.open}
          </Badge>
          <Badge
            variant='outline'
            className='border-sky-500/50 text-sky-700 dark:text-sky-300'
          >
            {t('Waiting for probe')} {totals.ready}
          </Badge>
          <Badge variant='secondary'>
            {t('Probing')} {totals.halfOpen}
          </Badge>
          <Badge
            variant='outline'
            className='border-amber-500/50 text-amber-700 dark:text-amber-300'
          >
            {t('Suspect')} {totals.suspect}
          </Badge>
          <Badge variant='outline'>
            {t('Healthy')} {totals.healthy}
          </Badge>
          <Badge variant='outline'>
            {t('Recovered')} {totals.recovered}
          </Badge>
        </div>
        <div className='flex min-w-0 items-center gap-2'>
          <Input
            value={modelFilter}
            onChange={(event) => {
              setModelFilter(event.target.value)
              setPage(1)
            }}
            placeholder={t('Filter by model...')}
            className='min-w-0 flex-1 sm:w-56 sm:flex-none'
          />
          <Select
            items={stateOptions.map((option) => ({
              ...option,
              label: t(option.label),
            }))}
            value={state}
            onValueChange={(value) => {
              setState(value ?? 'all')
              setPage(1)
            }}
          >
            <SelectTrigger className='w-36'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              {stateOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='outline'
                  size='icon'
                  aria-label={t('Refresh')}
                  onClick={() => healthQuery.refetch()}
                  disabled={healthQuery.isFetching}
                />
              }
            >
              <RefreshCw
                className={healthQuery.isFetching ? 'animate-spin' : undefined}
              />
            </TooltipTrigger>
            <TooltipContent>{t('Refresh')}</TooltipContent>
          </Tooltip>
        </div>
      </div>

      <div className='min-h-0 flex-1 overflow-auto'>
        <Table>
          <TableHeader className='bg-background sticky top-0 z-10'>
            <TableRow>
              <TableHead>{t('Channel')}</TableHead>
              <TableHead>{t('Group')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('State')}</TableHead>
              <TableHead>{t('Failures')}</TableHead>
              <TableHead>{t('Last failure')}</TableHead>
              <TableHead>{t('Cooldown until')}</TableHead>
              <TableHead>{t('Last error')}</TableHead>
              <TableHead>{t('Last request')}</TableHead>
              <TableHead className='w-12 text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>{tableRows}</TableBody>
        </Table>
      </div>

      <div className='flex items-center justify-between border-t pt-3'>
        <span className='text-muted-foreground text-sm'>
          {t('Total')}: {total}
        </span>
        <div className='flex items-center gap-2'>
          <span className='text-muted-foreground text-sm'>
            {page} / {pageCount}
          </span>
          <Button
            variant='outline'
            size='icon-sm'
            aria-label={t('Previous page')}
            disabled={page <= 1}
            onClick={() => setPage((value) => Math.max(1, value - 1))}
          >
            <ChevronLeft />
          </Button>
          <Button
            variant='outline'
            size='icon-sm'
            aria-label={t('Next page')}
            disabled={page >= pageCount}
            onClick={() => setPage((value) => Math.min(pageCount, value + 1))}
          >
            <ChevronRight />
          </Button>
        </div>
      </div>
    </div>
  )
}
