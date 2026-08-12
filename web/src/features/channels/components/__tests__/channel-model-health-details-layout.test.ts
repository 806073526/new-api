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

function readComponent(filename: string): Promise<string> {
  return readFile(resolve(componentsDirectory, '..', filename), 'utf8')
}

test('uses widened responsive popovers and a single-line last request', async () => {
  const [closedDetails, columns, lastRequest] = await Promise.all([
    readComponent('channel-model-health-closed-details-popover.tsx'),
    readComponent('channels-columns.tsx'),
    readComponent('channel-model-health-last-request.tsx'),
  ])
  const responsiveWidth =
    /w-\[34rem\] max-w-\[calc\(100vw-2rem\)\] overflow-auto/

  assert.match(closedDetails, responsiveWidth)
  assert.match(columns, responsiveWidth)
  assert.match(
    lastRequest,
    /className='text-muted-foreground text-xs whitespace-nowrap'/
  )
})

test('separates model detail rows in both popovers', async () => {
  const [closedDetails, columns] = await Promise.all([
    readComponent('channel-model-health-closed-details-popover.tsx'),
    readComponent('channels-columns.tsx'),
  ])
  const rowSpacing = /className='space-y-3'/

  assert.match(closedDetails, rowSpacing)
  assert.match(columns, rowSpacing)
  for (const source of [closedDetails, columns]) {
    assert.match(source, /border-b/)
    assert.match(source, /border-border\/60/)
    assert.match(source, /pb-3/)
    assert.match(source, /last:border-b-0/)
    assert.match(source, /last:pb-0/)
  }
})

test('does not render compact model labels below the health badges', async () => {
  const columns = await readComponent('channels-columns.tsx')

  assert.doesNotMatch(columns, /getChannelModelHealthCompactLabels/)
  assert.doesNotMatch(columns, /issueLabels/)
})

test('places healthy and recovered badges above active health states', async () => {
  const columns = await readComponent('channels-columns.tsx')

  assert.match(columns, /className='flex min-w-0 flex-col gap-1'/)
  assert.match(
    columns,
    /<div className='flex min-w-0 flex-wrap gap-1'>\s*\{healthy > 0/s
  )
  assert.match(
    columns,
    /\{active > 0 && \(\s*<div className='flex min-w-0 flex-wrap gap-1'>\s*\{issueContent/s
  )
})
