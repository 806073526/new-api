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
import type {
  ChannelModelHealth,
  ChannelModelHealthState,
  ChannelModelHealthSummaryModel,
} from '../types.ts'

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

export type ChannelModelHealthPresentationState =
  | 'healthy'
  | 'recovered'
  | 'suspect'
  | 'open'
  | 'ready'
  | 'half_open'

export function getChannelModelHealthPresentationState(
  item: ChannelModelHealth,
  now: number
): ChannelModelHealthPresentationState {
  const state = getChannelModelHealthDisplayState(item, now)
  if (state !== 'closed') {
    return state
  }
  return getChannelModelHealthClosedDisplayState(item)
}

export function getChannelModelHealthPresentationLabelKey(
  state: ChannelModelHealthPresentationState
): string {
  switch (state) {
    case 'healthy':
      return 'Healthy'
    case 'recovered':
      return 'Recovered'
    case 'suspect':
      return 'Suspect'
    case 'open':
      return 'Circuit open'
    case 'ready':
      return 'Waiting for probe'
    case 'half_open':
      return 'Probing'
  }
}

type ChannelModelHealthSummaryGroup = {
  state: ChannelModelHealthPresentationState
  group: string
  models: string[]
}

const channelModelHealthPresentationOrder: Record<
  ChannelModelHealthPresentationState,
  number
> = {
  healthy: 0,
  suspect: 1,
  recovered: 2,
  open: 3,
  ready: 4,
  half_open: 5,
}

function getChannelModelHealthSummaryPresentationState(
  item: ChannelModelHealthSummaryModel
): ChannelModelHealthPresentationState {
  if (item.state === 'closed') {
    return getChannelModelHealthClosedDisplayState(item)
  }
  if (item.state === 'open' && item.ready) {
    return 'ready'
  }
  return item.state
}

export function groupChannelModelHealthSummaryModels(
  items: ChannelModelHealthSummaryModel[]
): ChannelModelHealthSummaryGroup[] {
  const grouped = new Map<string, ChannelModelHealthSummaryGroup>()
  for (const item of items) {
    const state = getChannelModelHealthSummaryPresentationState(item)
    const key = `${state}\u0000${item.group}`
    const existing = grouped.get(key)
    if (existing) {
      existing.models.push(item.model)
      continue
    }
    grouped.set(key, {
      state,
      group: item.group,
      models: [item.model],
    })
  }

  return [...grouped.values()]
    .map((item) => ({
      ...item,
      models: [...new Set(item.models)].sort(),
    }))
    .sort((left, right) => {
      const stateOrder =
        channelModelHealthPresentationOrder[left.state] -
        channelModelHealthPresentationOrder[right.state]
      if (stateOrder !== 0) return stateOrder
      return left.group.localeCompare(right.group)
    })
}

export function hasMultipleChannelModelHealthGroups(
  items: Array<Pick<ChannelModelHealthSummaryGroup, 'group'>>
): boolean {
  return new Set(items.map((item) => item.group)).size > 1
}

export function getChannelModelHealthIssueLabels(
  issues: Array<{ model: string; group: string }>
): string[] {
  return issues.map((issue) =>
    issue.group ? `${issue.model} · ${issue.group}` : issue.model
  )
}

export type ChannelModelHealthLastRequest = {
  username: string
  tokenName: string
  requestAt: number
}

export function getChannelModelHealthLastRequest(
  item: Pick<
    ChannelModelHealth,
    'last_request_username' | 'last_request_token_name' | 'last_request_at'
  >
): ChannelModelHealthLastRequest | null {
  if (item.last_request_at <= 0) {
    return null
  }
  return {
    username: item.last_request_username,
    tokenName: item.last_request_token_name,
    requestAt: item.last_request_at,
  }
}
