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
import { readFile } from 'node:fs/promises'
import { describe, test } from 'node:test'

const zh = JSON.parse(
  await readFile(new URL('./locales/zh.json', import.meta.url), 'utf8')
) as { translation: Record<string, string> }

describe('channel upstream translations', () => {
  test('provides Chinese labels for upstream metrics and priority initialization', () => {
    assert.deepEqual(
      {
        ratio: zh.translation['Upstream Ratio'],
        balance: zh.translation['Upstream Balance'],
        action: zh.translation['Initialize Upstream Priorities'],
        title: zh.translation['Initialize upstream priorities?'],
        description:
          zh.translation[
            'This assigns priority 500 to the lowest upstream ratio and decreases priority by 10 for each distinct ratio tier. Only channels with synced upstream ratios are changed.'
          ],
        failure: zh.translation['Initialization failed'],
        success:
          zh.translation[
            'Upstream priorities initialized: {{updated}} updated'
          ],
      },
      {
        ratio: '上游倍率',
        balance: '上游余额',
        action: '初始化上游优先级',
        title: '初始化上游优先级？',
        description:
          '最低上游倍率的渠道优先级设为 500，每增加一个不同的倍率档位，优先级降低 10。只会修改已同步上游倍率的渠道。',
        failure: '初始化失败',
        success: '上游优先级初始化完成：已更新 {{updated}} 个渠道',
      }
    )
  })
})
