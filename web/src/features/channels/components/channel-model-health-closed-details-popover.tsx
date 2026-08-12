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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

import { getChannelModelHealth } from '../api'
import {
  getChannelModelHealthClosedItems,
  type ChannelModelHealthClosedDisplayState,
} from '../lib/channel-model-health'
import { ChannelModelHealthLastRequest } from './channel-model-health-last-request'

type ChannelModelHealthClosedDetailsPopoverProps = {
  channelId: number
  count: number
  state: ChannelModelHealthClosedDisplayState
}

export function ChannelModelHealthClosedDetailsPopover(
  props: ChannelModelHealthClosedDetailsPopoverProps
) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const detailsQuery = useQuery({
    queryKey: ['channel-model-health-closed-details', props.channelId],
    queryFn: () =>
      getChannelModelHealth({
        channel_id: props.channelId,
        page_size: 200,
        state: 'closed',
      }),
    enabled: open,
  })
  const title = props.state === 'healthy' ? t('Healthy') : t('Recovered')
  const items = getChannelModelHealthClosedItems(
    detailsQuery.data?.data?.items ?? [],
    props.state
  )
  const isRecovered = props.state === 'recovered'
  let detailContent = (
    <div className='text-muted-foreground text-xs'>{t('Loading...')}</div>
  )
  if (!detailsQuery.isLoading) {
    if (detailsQuery.isError || detailsQuery.data?.success === false) {
      detailContent = (
        <div className='text-muted-foreground text-xs'>
          {detailsQuery.data?.message || t('No model health records')}
        </div>
      )
    } else if (items.length === 0) {
      detailContent = (
        <div className='text-muted-foreground text-xs'>
          {t('No model health records')}
        </div>
      )
    } else {
      detailContent = (
        <>
          {items.map((item) => (
            <div
              key={`${item.group}:${item.model}`}
              className='border-border/60 border-b pb-3 text-xs last:border-b-0 last:pb-0'
            >
              <div className='font-mono'>{item.model}</div>
              <div className='text-muted-foreground'>
                {item.group} · {title}
              </div>
              <ChannelModelHealthLastRequest item={item} />
            </div>
          ))}
        </>
      )
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button
            type='button'
            className='focus-visible:ring-ring inline-flex min-w-0 cursor-pointer rounded-sm focus-visible:ring-2 focus-visible:outline-none'
            aria-label={`${title}: ${props.count}`}
          />
        }
      >
        {isRecovered ? (
          <Badge
            variant='outline'
            className='border-emerald-500/50 text-emerald-700 dark:text-emerald-300'
          >
            {title} {props.count}
          </Badge>
        ) : (
          <Badge variant='outline'>
            {title} {props.count}
          </Badge>
        )}
      </PopoverTrigger>
      <PopoverContent className='max-h-96 w-[34rem] max-w-[calc(100vw-2rem)] overflow-auto'>
        <div className='space-y-3'>
          <div className='text-muted-foreground text-xs'>
            {title} {props.count}
          </div>
          {detailContent}
        </div>
      </PopoverContent>
    </Popover>
  )
}
