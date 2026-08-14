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

test('translates channel model health controls in simplified Chinese', async () => {
  const locale = JSON.parse(
    await readFile(
      resolve(
        componentsDirectory,
        '..',
        '..',
        '..',
        '..',
        'i18n',
        'locales',
        'zh.json'
      ),
      'utf8'
    )
  ) as { translation?: Record<string, string> }
  const translations = locale.translation ?? {}
  const expected: Record<string, string> = {
    'Manually disabled': '手动禁用',
    'Manually disable model': '手动禁用模型',
    'Restore manually disabled model': '恢复手动禁用的模型',
    'Model manually disabled': '模型已手动禁用',
    'Manually disabled model restored': '手动禁用的模型已恢复',
    'Failed to manually disable model': '手动禁用模型失败',
    'Failed to restore manually disabled model': '恢复手动禁用模型失败',
    'Disable model': '禁用模型',
    'Restore model': '恢复模型',
    'Reason (optional)': '原因（可选）',
    'This blocks the selected channel model for every group. Automatic probes and successes cannot restore it.':
      '这会阻止所选渠道模型的所有分组。自动探测和成功请求都无法恢复它。',
    'This immediately makes the selected channel model available again and resets its automatic health state.':
      '这会立即恢复所选渠道模型的可用性，并重置其自动健康状态。',
    'This blocks {{model}} for every group of this channel. Automatic probes and successes cannot restore it.':
      '这会阻止该渠道所有分组中的 {{model}}。自动探测和成功请求都无法恢复它。',
    'This immediately makes {{model}} available again and resets its automatic health state.':
      '这会立即恢复 {{model}} 的可用性，并重置其自动健康状态。',
    Operator: '操作人',
  }

  for (const [key, value] of Object.entries(expected)) {
    assert.equal(
      translations[key],
      value,
      `missing Chinese translation: ${key}`
    )
  }
})
