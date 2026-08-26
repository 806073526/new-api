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

export const channelAvailabilityLayoutClasses = {
  panel: 'w-full min-w-0',
  groups:
    'grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(min(100%,32rem),1fr))]',
  modelName: 'min-w-0 break-words font-mono text-sm leading-5',
} as const
