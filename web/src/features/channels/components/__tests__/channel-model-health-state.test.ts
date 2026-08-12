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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import * as channelModelHealth from '../../lib/channel-model-health.ts'
import type { ChannelModelHealth } from '../../types'

const baseHealth: ChannelModelHealth = {
  id: 1,
  channel_id: 27,
  group: 'default',
  model: 'gpt-test',
  state: 'open',
  failure_count: 3,
  cooldown_until: 200,
  half_open_lease_until: 0,
  last_failure_at: 100,
  last_success_at: 0,
  last_status_code: 502,
  last_error_code: 'upstream_error',
  last_error: 'upstream unavailable',
  last_request_username: '',
  last_request_token_name: '',
  last_request_at: 0,
  updated_at: 100,
  channel_name: 'test-channel',
  channel_status: 1,
}

describe('channel model health display state', () => {
  test('shows an expired circuit as waiting for probe without changing active circuits', () => {
    assert.equal(
      channelModelHealth.getChannelModelHealthDisplayState(baseHealth, 199),
      'open'
    )
    assert.equal(
      channelModelHealth.getChannelModelHealthDisplayState(baseHealth, 200),
      'ready'
    )
    assert.equal(
      channelModelHealth.getChannelModelHealthDisplayState(
        { ...baseHealth, state: 'half_open' },
        200
      ),
      'half_open'
    )
  })

  test('returns model and group labels for problematic channel models', () => {
    assert.deepEqual(
      channelModelHealth.getChannelModelHealthIssueLabels([
        { model: 'gpt-5.6-luna', group: 'stable' },
        { model: 'claude-test', group: 'backup' },
      ]),
      ['gpt-5.6-luna · stable', 'claude-test · backup']
    )
  })

  test('does not expose a last request when no matching request log exists', () => {
    const getLastRequest = Reflect.get(
      channelModelHealth,
      'getChannelModelHealthLastRequest'
    )
    assert.equal(typeof getLastRequest, 'function')
    assert.equal(getLastRequest(baseHealth), null)
  })

  test('returns the latest requester details when a matching request log exists', () => {
    const getLastRequest = Reflect.get(
      channelModelHealth,
      'getChannelModelHealthLastRequest'
    )
    assert.equal(typeof getLastRequest, 'function')
    assert.deepEqual(
      getLastRequest({
        ...baseHealth,
        last_request_username: 'company-user',
        last_request_token_name: 'company-token',
        last_request_at: 1_754_000_000,
      }),
      {
        username: 'company-user',
        tokenName: 'company-token',
        requestAt: 1_754_000_000,
      }
    )
  })

  test('distinguishes never-observed healthy models from recovered models', () => {
    assert.equal(
      channelModelHealth.getChannelModelHealthClosedDisplayState({
        health_record_exists: false,
      }),
      'healthy'
    )
    assert.equal(
      channelModelHealth.getChannelModelHealthClosedDisplayState({
        health_record_exists: true,
      }),
      'recovered'
    )
  })

  test('returns only never-observed models for healthy detail', () => {
    const getClosedItems = Reflect.get(
      channelModelHealth,
      'getChannelModelHealthClosedItems'
    )
    assert.equal(typeof getClosedItems, 'function')
    assert.deepEqual(
      getClosedItems(
        [
          {
            ...baseHealth,
            model: 'healthy-model',
            state: 'closed',
            health_record_exists: false,
          },
          {
            ...baseHealth,
            model: 'recovered-model',
            state: 'closed',
            health_record_exists: true,
          },
          {
            ...baseHealth,
            model: 'open-model',
            state: 'open',
            health_record_exists: false,
          },
        ],
        'healthy'
      ).map((item: ChannelModelHealth) => item.model),
      ['healthy-model']
    )
  })

  test('returns only persisted records for recovered detail', () => {
    const getClosedItems = Reflect.get(
      channelModelHealth,
      'getChannelModelHealthClosedItems'
    )
    assert.equal(typeof getClosedItems, 'function')
    assert.deepEqual(
      getClosedItems(
        [
          {
            ...baseHealth,
            model: 'healthy-model',
            state: 'closed',
            health_record_exists: false,
          },
          {
            ...baseHealth,
            model: 'recovered-model',
            state: 'closed',
            health_record_exists: true,
          },
          {
            ...baseHealth,
            model: 'open-model',
            state: 'open',
            health_record_exists: true,
          },
        ],
        'recovered'
      ).map((item: ChannelModelHealth) => item.model),
      ['recovered-model']
    )
  })

  test('groups every model by state and group for the channel list', () => {
    const groupSummaryModels = Reflect.get(
      channelModelHealth,
      'groupChannelModelHealthSummaryModels'
    )
    assert.equal(typeof groupSummaryModels, 'function')
    assert.deepEqual(
      groupSummaryModels([
        {
          group: 'A',
          model: 'gpt-5.6-sol',
          state: 'closed',
          ready: false,
          health_record_exists: false,
        },
        {
          group: 'A',
          model: 'gpt-5.5',
          state: 'open',
          ready: false,
          health_record_exists: true,
        },
        {
          group: 'A',
          model: 'gpt-5.4',
          state: 'open',
          ready: false,
          health_record_exists: true,
        },
        {
          group: 'B',
          model: 'gpt-5.6-sol',
          state: 'open',
          ready: false,
          health_record_exists: true,
        },
        {
          group: 'B',
          model: 'gpt-5.6-luna',
          state: 'suspect',
          ready: false,
          health_record_exists: true,
        },
        {
          group: 'C',
          model: 'gpt-4.1',
          state: 'closed',
          ready: false,
          health_record_exists: true,
        },
      ]),
      [
        { state: 'healthy', group: 'A', models: ['gpt-5.6-sol'] },
        { state: 'suspect', group: 'B', models: ['gpt-5.6-luna'] },
        { state: 'recovered', group: 'C', models: ['gpt-4.1'] },
        { state: 'open', group: 'A', models: ['gpt-5.4', 'gpt-5.5'] },
        { state: 'open', group: 'B', models: ['gpt-5.6-sol'] },
      ]
    )
  })

  test('shows group labels only when the health display has multiple groups', () => {
    const hasMultipleGroups = Reflect.get(
      channelModelHealth,
      'hasMultipleChannelModelHealthGroups'
    )
    assert.equal(typeof hasMultipleGroups, 'function')
    assert.equal(
      hasMultipleGroups([{ group: 'default' }, { group: 'default' }]),
      false
    )
    assert.equal(
      hasMultipleGroups([{ group: 'alpha' }, { group: 'beta' }]),
      true
    )
  })
})
