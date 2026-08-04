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

export function formatChannelLogCount(summary: {
  logs: number
  users: number
}): string {
  return `${summary.logs} / ${summary.users}`
}

export function formatChannelActivityIdentity(detail: {
  username: string
  token_name: string
}): string {
  if (!detail.token_name) {
    return detail.username
  }
  return `${detail.username} · ${detail.token_name}`
}

export function getTopChannelActivityDetails<
  T extends {
    logs: number
    last_created_at: number
  },
>(details: T[], limit = 10): T[] {
  return [...details]
    .sort(
      (left, right) =>
        right.logs - left.logs || right.last_created_at - left.last_created_at
    )
    .slice(0, limit)
}
