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

test('provides persistent manual model disable and recovery controls', async () => {
  const source = await readFile(
    resolve(componentsDirectory, '..', 'channel-model-health.tsx'),
    'utf8'
  )

  assert.match(source, /disableChannelModelManually/)
  assert.match(source, /recoverChannelModelManuallyDisabled/)
  assert.match(source, /Automatic probes and successes cannot restore it/)
  assert.match(source, /Reason \(optional\)/)
})
