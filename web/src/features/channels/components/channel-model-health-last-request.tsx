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
import { useTranslation } from 'react-i18next'

import { formatTimestamp } from '@/lib/format'

import { getChannelModelHealthLastRequest } from '../lib/channel-model-health'
import type { ChannelModelHealth } from '../types'

type ChannelModelHealthLastRequestProps = {
  item: Pick<
    ChannelModelHealth,
    'last_request_username' | 'last_request_token_name' | 'last_request_at'
  >
}

export function ChannelModelHealthLastRequest(
  props: ChannelModelHealthLastRequestProps
) {
  const { t } = useTranslation()
  const lastRequest = getChannelModelHealthLastRequest(props.item)
  if (!lastRequest) {
    return null
  }

  return (
    <div className='text-muted-foreground text-xs whitespace-nowrap'>
      {t('Last request: {{username}} user {{tokenName}} token {{time}}', {
        username: lastRequest.username || '-',
        tokenName: lastRequest.tokenName || '-',
        time: formatTimestamp(lastRequest.requestAt),
      })}
    </div>
  )
}
