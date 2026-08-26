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

import { channelAvailabilityLayoutClasses } from '../channel-availability-layout.ts'

describe('channel availability layout', () => {
  test('uses the full dashboard width and responsive group columns', () => {
    assert.match(channelAvailabilityLayoutClasses.panel, /w-full/)
    assert.match(channelAvailabilityLayoutClasses.groups, /auto-fit/)
    assert.match(channelAvailabilityLayoutClasses.groups, /minmax/)
  })

  test('keeps long model names visible instead of truncating them', () => {
    assert.match(channelAvailabilityLayoutClasses.modelName, /break-words/)
    assert.doesNotMatch(channelAvailabilityLayoutClasses.modelName, /truncate/)
  })
})
