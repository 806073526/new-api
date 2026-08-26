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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  formatChannelAvailabilityPerformance,
  formatChannelAvailabilityResponseTime,
  getChannelAvailabilityReasonKey,
  getChannelAvailabilitySourceKey,
} from '../channel-availability-display'

describe('channel availability display metadata', () => {
  test('maps observation sources to user-facing translation keys', () => {
    assert.equal(getChannelAvailabilitySourceKey('active_test'), 'Active test')
    assert.equal(getChannelAvailabilitySourceKey('request'), 'Recent request')
    assert.equal(getChannelAvailabilitySourceKey('legacy'), 'Unknown source')
  })

  test('explains why an unknown model has no current status', () => {
    assert.equal(
      getChannelAvailabilityReasonKey('no_observation'),
      'No recent observation'
    )
    assert.equal(
      getChannelAvailabilityReasonKey('stale'),
      'Observation expired'
    )
    assert.equal(
      getChannelAvailabilityReasonKey('recent_failure'),
      'Recent observation failed'
    )
    assert.equal(
      getChannelAvailabilityReasonKey('unexpected'),
      'No recent observation'
    )
  })

  test('formats model response time without showing invalid values', () => {
    assert.equal(formatChannelAvailabilityResponseTime(820), '820 ms')
    assert.equal(formatChannelAvailabilityResponseTime(1250), '1.3 s')
    assert.equal(formatChannelAvailabilityResponseTime(0), undefined)
    assert.equal(formatChannelAvailabilityResponseTime(Number.NaN), undefined)
  })

  test('formats model performance metrics in compact display order', () => {
    assert.deepEqual(
      formatChannelAvailabilityPerformance({
        avg_tps: 67.2,
        avg_ttft_ms: 3210,
        avg_latency_ms: 21610,
        success_rate: 90,
      }),
      [
        { key: 'tps', value: '67.2 t/s' },
        { key: 'ttft', value: '3.21s' },
        { key: 'latency', value: '21.61s' },
        { key: 'successRate', value: '90.00%' },
      ]
    )
  })

  test('omits unavailable model performance metrics', () => {
    assert.deepEqual(
      formatChannelAvailabilityPerformance({
        avg_tps: 0,
        avg_ttft_ms: 0,
        avg_latency_ms: Number.NaN,
        success_rate: Number.NaN,
      }),
      []
    )
  })
})
