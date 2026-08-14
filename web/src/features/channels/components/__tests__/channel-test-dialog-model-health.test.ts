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
*/
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

const componentsDirectory = dirname(fileURLToPath(import.meta.url))

test('observes dialog model tests and provides model health controls', async () => {
  const source = await readFile(
    resolve(componentsDirectory, '..', 'dialogs', 'channel-test-dialog.tsx'),
    'utf8'
  )

  assert.match(source, /getChannelModelHealth/)
  assert.match(source, /observeHealth:\s*true/)
  assert.match(source, /id:\s*'model_health'/)
  assert.match(source, /openChannelModelHealth/)
  assert.match(source, /recoverChannelModelHealth/)
})

test('refreshes health summaries when a channel test refreshes the channel list', async () => {
  const source = await readFile(
    resolve(componentsDirectory, '..', 'dialogs', 'channel-test-dialog.tsx'),
    'utf8'
  )
  const refreshStart = source.indexOf('const refreshChannelLists')
  const testStart = source.indexOf('const testSingleModel', refreshStart)
  assert.ok(refreshStart >= 0)
  assert.ok(testStart > refreshStart)
  const refreshSource = source.slice(refreshStart, testStart)
  assert.match(refreshSource, /invalidateQueries\(/)
  assert.match(
    refreshSource,
    /queryKey:\s*\[\s*['"]channel-model-health-summary['"]\s*\]/
  )
})
