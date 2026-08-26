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
import {
  Activity,
  CircleCheck,
  CircleHelp,
  CircleX,
  Play,
  RefreshCw,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { ModelDetailsDrawer } from '@/features/pricing/components'
import { DEFAULT_TOKEN_UNIT } from '@/features/pricing/constants'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import {
  getSuccessRateDotClass,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import type { PerfModelSummary } from '@/features/performance-metrics/types'
import {
  getChannelAvailability,
  getChannelAvailabilityTestStatus,
  startChannelAvailabilityTest,
} from '@/features/dashboard/api'
import type {
  ChannelAvailabilityGroup,
  ChannelAvailabilityModel,
  ChannelAvailabilitySnapshot,
} from '@/features/dashboard/types'
import { cn } from '@/lib/utils'

import {
  formatChannelAvailabilityPerformance,
  formatChannelAvailabilityResponseTime,
  getChannelAvailabilityReasonKey,
  getChannelAvailabilitySourceKey,
} from './channel-availability-display'
import { channelAvailabilityLayoutClasses } from './channel-availability-layout'

const AVAILABILITY_QUERY_KEY = ['dashboard', 'channel-availability']
const TEST_STATUS_QUERY_KEY = ['dashboard', 'channel-availability-test']
const ACTIVE_TASK_STATUSES = new Set(['pending', 'running'])

const EMPTY_SNAPSHOT: ChannelAvailabilitySnapshot = {
  groups: [],
  freshness_seconds: 0,
  generated_at: 0,
}

const STATE_STYLE = {
  available: {
    icon: CircleCheck,
    dot: 'bg-emerald-500',
    text: 'text-emerald-700 dark:text-emerald-300',
    label: 'Available',
  },
  unavailable: {
    icon: CircleX,
    dot: 'bg-red-500',
    text: 'text-red-700 dark:text-red-300',
    label: 'Unavailable',
  },
  unknown: {
    icon: CircleHelp,
    dot: 'bg-amber-500',
    text: 'text-amber-700 dark:text-amber-300',
    label: 'Unknown',
  },
} as const

const PERFORMANCE_LABELS = {
  tps: 'TPS',
  ttft: 'TTFT',
  latency: 'Latency short',
  successRate: 'Success rate',
} as const

function SuccessRateBars(props: { performance?: PerfModelSummary }) {
  if (!props.performance || !Number.isFinite(props.performance.success_rate)) {
    return null
  }

  const recentRates =
    props.performance.recent_success_rates?.filter((rate) =>
      Number.isFinite(rate)
    ) ?? []
  const statusRates =
    recentRates.length > 0
      ? recentRates.slice(-3)
      : [props.performance.success_rate]
  const statusBars = [
    ...Array(Math.max(0, 3 - statusRates.length)).fill(null),
    ...statusRates,
  ].slice(-3)

  return (
    <span
      className='inline-flex h-3.5 items-end gap-0.5'
      title={`${props.performance.success_rate.toFixed(2)}%`}
      aria-label={`${props.performance.success_rate.toFixed(2)}%`}
    >
      {statusBars.map((rate, index) => (
        <span
          key={`${index}-${rate ?? 'empty'}`}
          className={cn(
            'w-1 rounded-full',
            index === 0 && 'h-2',
            index === 1 && 'h-2.5',
            index === 2 && 'h-3.5',
            rate == null
              ? 'bg-muted-foreground/15'
              : getSuccessRateDotClass(rate)
          )}
        />
      ))}
    </span>
  )
}

function ModelAvailabilityRow(props: {
  model: ChannelAvailabilityModel
  performance?: PerfModelSummary
  onModelClick?: (modelName: string) => void
}) {
  const { t } = useTranslation()
  const style = STATE_STYLE[props.model.state]
  const Icon = style.icon
  const responseTime = formatChannelAvailabilityResponseTime(
    props.model.latency_ms
  )
  const source = t(getChannelAvailabilitySourceKey(props.model.source))
  const reason = t(getChannelAvailabilityReasonKey(props.model.reason))

  const detailParts = props.model.state === 'available' ? [] : [reason]
  detailParts.push(source)
  if (responseTime) {
    detailParts.push(responseTime)
  }
  const detail = detailParts.join(' · ')
  const performanceMetrics = formatChannelAvailabilityPerformance(
    props.performance
  )
  const isClickable = Boolean(props.onModelClick)

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (!isClickable || !props.onModelClick) return
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      props.onModelClick(props.model.model)
    }
  }

  return (
    <div
      className={cn(
        'grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-start gap-3 border-t px-4 py-3 first:border-t-0 sm:px-5',
        isClickable &&
          'cursor-pointer transition-colors hover:bg-muted/30 focus-visible:bg-muted/30 focus-visible:outline-none'
      )}
      role={isClickable ? 'button' : undefined}
      tabIndex={isClickable ? 0 : undefined}
      onClick={
        isClickable ? () => props.onModelClick?.(props.model.model) : undefined
      }
      onKeyDown={isClickable ? handleKeyDown : undefined}
      aria-label={isClickable ? `${t('Model details')}: ${props.model.model}` : undefined}
    >
      <div className='min-w-0'>
        <div className='flex min-w-0 items-center gap-2.5'>
        <span
          className={cn('size-2 shrink-0 rounded-full', style.dot)}
          aria-hidden='true'
        />
        <span
          className={channelAvailabilityLayoutClasses.modelName}
          title={props.model.model}
        >
          {props.model.model}
        </span>
        </div>
        {performanceMetrics.length > 0 && (
          <div className='mt-1 flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-0.5 pl-[18px] text-[10px] leading-4 tabular-nums'>
            {performanceMetrics.map((metric) => (
              <span
                key={metric.key}
                className={cn(
                  'inline-flex items-center gap-1 whitespace-nowrap',
                  metric.key === 'successRate' &&
                    getSuccessRateTextClass(props.performance?.success_rate ?? Number.NaN)
                )}
              >
                <span className='text-muted-foreground/70'>
                  {t(PERFORMANCE_LABELS[metric.key])}
                </span>
                <span className='font-mono'>{metric.value}</span>
                {metric.key === 'successRate' && (
                  <SuccessRateBars performance={props.performance} />
                )}
              </span>
            ))}
          </div>
        )}
      </div>
      <div className='flex min-w-0 shrink-0 flex-col items-end gap-0.5'>
        <span
          className={cn(
            'inline-flex items-center gap-1.5 text-xs font-medium',
            style.text
          )}
        >
          <Icon className='size-3.5' aria-hidden='true' />
          {t(style.label)}
        </span>
        <span className='text-muted-foreground max-w-52 text-right text-[11px] leading-4 break-words'>
          {detail}
        </span>
      </div>
    </div>
  )
}

function GroupAvailabilityCard(props: {
  group: ChannelAvailabilityGroup
  performanceByModel: Map<string, PerfModelSummary>
  onModelClick?: (modelName: string) => void
}) {
  const { t } = useTranslation()

  return (
    <section className='bg-card min-w-0 overflow-hidden rounded-xl border shadow-xs'>
      <div className='bg-muted/30 flex items-start justify-between gap-3 border-b px-4 py-3 sm:px-5'>
        <h4
          className='min-w-0 text-sm leading-5 font-semibold break-words'
          title={props.group.group}
        >
          {props.group.group}
        </h4>
        <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
          {t('{{count}} models', { count: props.group.models.length })}
        </span>
      </div>
      <div>
        {props.group.models.map((model) => (
          <ModelAvailabilityRow
            key={`${props.group.group}:${model.model}`}
            model={model}
            performance={props.performanceByModel.get(model.model)}
            onModelClick={props.onModelClick}
          />
        ))}
      </div>
    </section>
  )
}

function LoadingGroups() {
  return (
    <div className={channelAvailabilityLayoutClasses.groups}>
      {[0, 1, 2, 3].map((item) => (
        <div key={item} className='bg-muted/40 h-24 animate-pulse rounded-xl' />
      ))}
    </div>
  )
}

export function ChannelAvailabilityPanel(props: { isAdmin: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedModelName, setSelectedModelName] = useState<string | null>(
    null
  )
  const {
    models: pricingModels,
    groupRatio,
    usableGroup,
    endpointMap,
    autoGroups,
    priceRate,
    usdExchangeRate,
  } = usePricingData()
  const availabilityQuery = useQuery({
    queryKey: AVAILABILITY_QUERY_KEY,
    queryFn: async () => {
      const response = await getChannelAvailability()
      if (!response.success) {
        throw new Error(response.message || t('Failed to load'))
      }
      return response.data ?? EMPTY_SNAPSHOT
    },
    staleTime: 30 * 1000,
    refetchInterval: 60 * 1000,
    retry: false,
  })
  const performanceQuery = useQuery({
    queryKey: ['perf-metrics-summary', 24],
    queryFn: () => getPerfMetricsSummary(24),
    staleTime: 60 * 1000,
    refetchInterval: 60 * 1000,
    retry: false,
  })
  const testStatusQuery = useQuery({
    queryKey: TEST_STATUS_QUERY_KEY,
    queryFn: async () => {
      const response = await getChannelAvailabilityTestStatus()
      if (!response.success) {
        throw new Error(response.message || t('Failed to load'))
      }
      return response.data ?? null
    },
    enabled: props.isAdmin,
    staleTime: 0,
    refetchInterval: (query) =>
      ACTIVE_TASK_STATUSES.has(query.state.data?.status ?? '') ? 2000 : false,
    retry: false,
  })
  const testMutation = useMutation({
    mutationFn: startChannelAvailabilityTest,
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Failed to start channel test'))
        return
      }
      void testStatusQuery.refetch()
      toast.success(t('Channel test started'))
    },
    onError: (error: Error) => {
      const responseMessage = (
        error as Error & {
          response?: { data?: { message?: string } }
        }
      ).response?.data?.message
      toast.error(
        responseMessage || error.message || t('Failed to start channel test')
      )
    },
  })

  const activeTask = testStatusQuery.data
  const isTesting =
    testMutation.isPending || ACTIVE_TASK_STATUSES.has(activeTask?.status ?? '')

  const wasTestingRef = useRef(false)
  useEffect(() => {
    if (wasTestingRef.current && !isTesting) {
      void queryClient.invalidateQueries({ queryKey: AVAILABILITY_QUERY_KEY })
    }
    wasTestingRef.current = isTesting
  }, [isTesting, queryClient])

  const handleRefresh = () => {
    void availabilityQuery.refetch()
  }

  const handleTestAll = () => {
    if (!isTesting) {
      testMutation.mutate()
    }
  }

  const snapshot = availabilityQuery.data ?? EMPTY_SNAPSHOT
  const performanceByModel = useMemo(() => {
    const map = new Map<string, PerfModelSummary>()
    for (const model of performanceQuery.data?.data?.models ?? []) {
      map.set(model.model_name, model)
    }
    return map
  }, [performanceQuery.data])
  const selectedModel = useMemo(() => {
    if (!selectedModelName) return null
    return (
      pricingModels.find((model) => model.model_name === selectedModelName) ??
      {
        id: 0,
        model_name: selectedModelName,
        quota_type: 0,
        model_ratio: 1,
        completion_ratio: 1,
        enable_groups: [],
      }
    )
  }, [pricingModels, selectedModelName])
  const hasGroups = snapshot.groups.length > 0
  let availabilityContent
  if (availabilityQuery.isLoading) {
    availabilityContent = <LoadingGroups />
  } else if (availabilityQuery.isError) {
    availabilityContent = (
      <ErrorState
        title={t('Failed to load channel availability')}
        description={
          availabilityQuery.error instanceof Error
            ? availabilityQuery.error.message
            : undefined
        }
        onRetry={handleRefresh}
      />
    )
  } else if (!hasGroups) {
    availabilityContent = (
      <div className='text-muted-foreground rounded-xl border border-dashed px-4 py-10 text-center text-sm'>
        {t('No channel models configured')}
      </div>
    )
  } else {
    availabilityContent = (
      <div className={channelAvailabilityLayoutClasses.groups}>
        {snapshot.groups.map((group) => (
          <GroupAvailabilityCard
            key={group.group}
            group={group}
            performanceByModel={performanceByModel}
            onModelClick={setSelectedModelName}
          />
        ))}
      </div>
    )
  }

  return (
    <section
      className={cn('space-y-3', channelAvailabilityLayoutClasses.panel)}
    >
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='flex min-w-0 items-center gap-2'>
          <IconBadge tone='success' size='sm'>
            <Activity />
          </IconBadge>
          <div className='min-w-0'>
            <h3 className='text-sm font-semibold'>
              {t('Channel availability')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t('Current model availability by group')}
            </p>
          </div>
        </div>
        <div className='flex shrink-0 items-center gap-1.5'>
          {props.isAdmin && (
            <Button
              variant='outline'
              size='sm'
              onClick={handleTestAll}
              disabled={isTesting}
              aria-label={t('Test All Channels')}
            >
              <Play data-icon='inline-start' />
              {isTesting ? t('Testing...') : t('Test All Channels')}
            </Button>
          )}
          <Button
            variant='ghost'
            size='icon'
            onClick={handleRefresh}
            disabled={availabilityQuery.isFetching}
            aria-label={t('Refresh')}
            className='size-8'
          >
            <RefreshCw
              className={cn(
                'size-3.5',
                availabilityQuery.isFetching && 'animate-spin'
              )}
              aria-hidden='true'
            />
          </Button>
        </div>
      </div>

      {props.isAdmin && isTesting && (
        <div
          className='text-muted-foreground flex items-center gap-2 text-xs'
          aria-live='polite'
        >
          <span className='size-1.5 animate-pulse rounded-full bg-sky-500' />
          {t('Testing all channel models...')}
        </div>
      )}

      {availabilityContent}

      {selectedModel && (
        <ModelDetailsDrawer
          open={Boolean(selectedModel)}
          onOpenChange={(open) => {
            if (!open) setSelectedModelName(null)
          }}
          model={selectedModel}
          groupRatio={groupRatio || {}}
          usableGroup={usableGroup || {}}
          endpointMap={
            (endpointMap as Record<
              string,
              { path?: string; method?: string }
            >) || {}
          }
          autoGroups={autoGroups || []}
          priceRate={priceRate ?? 1}
          usdExchangeRate={usdExchangeRate ?? 1}
          tokenUnit={DEFAULT_TOKEN_UNIT}
        />
      )}
    </section>
  )
}
