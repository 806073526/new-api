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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import * as channelActivity from '../../lib/channel-activity.ts'

describe('channel log display', () => {
  test('formats recent log and unique user counts', () => {
    assert.equal(
      channelActivity.formatChannelLogCount({ logs: 8, users: 3 }),
      '8 / 3'
    )
  })

  test('formats the username and token name without exposing an upstream key', () => {
    assert.equal(
      channelActivity.formatChannelActivityIdentity({
        username: 'company',
        token_name: 'company-key',
      }),
      'company · company-key'
    )
  })

  test('sorts activity details by log count and keeps only the top ten', () => {
    const details = Array.from({ length: 12 }, (_, index) => {
      if (index === 10) {
        return {
          username: `user-${index}`,
          token_name: `token-${index}`,
          logs: 20,
          last_created_at: 200,
        }
      }
      if (index === 11) {
        return {
          username: `user-${index}`,
          token_name: `token-${index}`,
          logs: 20,
          last_created_at: 100,
        }
      }
      return {
        username: `user-${index}`,
        token_name: `token-${index}`,
        logs: index,
        last_created_at: index,
      }
    })

    const getTopChannelActivityDetails = Reflect.get(
      channelActivity,
      'getTopChannelActivityDetails'
    )
    assert.equal(typeof getTopChannelActivityDetails, 'function')
    const topDetails = getTopChannelActivityDetails(details)

    assert.equal(topDetails.length, 10)
    assert.deepEqual(
      topDetails.slice(0, 2).map((detail) => detail.username),
      ['user-10', 'user-11']
    )
    assert.equal(topDetails.at(-1)?.username, 'user-2')
  })
})
