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
*/
import {
  formatLatency,
  formatThroughput,
  formatUptimePct,
} from '@/features/performance-metrics/lib/format'

export type ChannelAvailabilityPerformance = {
  avg_tps?: number
  avg_ttft_ms?: number
  avg_latency_ms?: number
  success_rate?: number
}

export type ChannelAvailabilityPerformanceMetric = {
  key: 'tps' | 'ttft' | 'latency' | 'successRate'
  value: string
}

function isFiniteNumber(value: number | undefined): value is number {
  return value !== undefined && Number.isFinite(value)
}

export function formatChannelAvailabilityPerformance(
  performance: ChannelAvailabilityPerformance | undefined
): ChannelAvailabilityPerformanceMetric[] {
  if (!performance) return []

  const metrics: ChannelAvailabilityPerformanceMetric[] = []
  const avgTps = performance.avg_tps
  const avgTtftMs = performance.avg_ttft_ms
  const avgLatencyMs = performance.avg_latency_ms
  const successRate = performance.success_rate

  if (isFiniteNumber(avgTps) && avgTps > 0) {
    metrics.push({
      key: 'tps',
      value: formatThroughput(avgTps),
    })
  }
  if (isFiniteNumber(avgTtftMs) && avgTtftMs > 0) {
    metrics.push({
      key: 'ttft',
      value: formatLatency(avgTtftMs),
    })
  }
  if (isFiniteNumber(avgLatencyMs) && avgLatencyMs > 0) {
    metrics.push({
      key: 'latency',
      value: formatLatency(avgLatencyMs),
    })
  }
  if (isFiniteNumber(successRate)) {
    metrics.push({
      key: 'successRate',
      value: formatUptimePct(successRate),
    })
  }
  return metrics
}

export function getChannelAvailabilitySourceKey(source: string | undefined) {
  switch (source) {
    case 'active_test':
      return 'Active test'
    case 'request':
      return 'Recent request'
    default:
      return 'Unknown source'
  }
}

export function getChannelAvailabilityReasonKey(reason: string | undefined) {
  switch (reason) {
    case 'stale':
      return 'Observation expired'
    case 'recent_failure':
      return 'Recent observation failed'
    case 'channel_disabled':
      return 'All channels are disabled'
    case 'temporarily_blocked':
      return 'All channels are temporarily blocked'
    case 'no_candidate':
      return 'No available channel'
    default:
      return 'No recent observation'
  }
}

export function formatChannelAvailabilityResponseTime(
  latencyMs: number | undefined
) {
  if (!Number.isFinite(latencyMs) || !latencyMs || latencyMs < 0) {
    return undefined
  }
  if (latencyMs < 1000) {
    return `${Math.round(latencyMs)} ms`
  }
  const seconds = (latencyMs / 1000).toFixed(1).replace(/\.0$/, '')
  return `${seconds} s`
}
