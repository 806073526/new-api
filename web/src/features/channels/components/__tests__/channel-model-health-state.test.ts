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

  test('returns unique model labels without groups for the channel list', () => {
    const getCompactLabels = Reflect.get(
      channelModelHealth,
      'getChannelModelHealthCompactLabels'
    )
    assert.equal(typeof getCompactLabels, 'function')
    assert.deepEqual(
      getCompactLabels([
        { model: 'gpt-5.6-luna', group: 'stable' },
        { model: 'gpt-5.6-luna', group: 'backup' },
        { model: 'claude-test', group: 'backup' },
      ]),
      ['gpt-5.6-luna', 'claude-test']
    )
  })
})
