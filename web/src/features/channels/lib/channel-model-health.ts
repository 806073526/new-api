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
import type { ChannelModelHealth, ChannelModelHealthState } from '../types.ts'

export type ChannelModelHealthDisplayState = ChannelModelHealthState | 'ready'

export function getChannelModelHealthDisplayState(
  item: ChannelModelHealth,
  now: number
): ChannelModelHealthDisplayState {
  if (item.state === 'open' && item.cooldown_until <= now) {
    return 'ready'
  }
  return item.state
}

export type ChannelModelHealthClosedDisplayState = 'healthy' | 'recovered'

export function getChannelModelHealthClosedDisplayState(
  item: Pick<ChannelModelHealth, 'health_record_exists'>
): ChannelModelHealthClosedDisplayState {
  return item.health_record_exists === false ? 'healthy' : 'recovered'
}

export function getChannelModelHealthClosedItems(
  items: ChannelModelHealth[],
  closedState: ChannelModelHealthClosedDisplayState
): ChannelModelHealth[] {
  return items.filter(
    (item) =>
      item.state === 'closed' &&
      getChannelModelHealthClosedDisplayState(item) === closedState
  )
}

export function getChannelModelHealthIssueLabels(
  issues: Array<{ model: string; group: string }>
): string[] {
  return issues.map((issue) =>
    issue.group ? `${issue.model} · ${issue.group}` : issue.model
  )
}

export function getChannelModelHealthCompactLabels(
  issues: Array<{ model: string; group?: string }>
): string[] {
  return [...new Set(issues.map((issue) => issue.model).filter(Boolean))]
}
