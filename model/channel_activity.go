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
*/
package model

import (
	"fmt"
	"sort"
	"time"
)

const (
	ChannelActivityWindowSeconds int64 = 60
	ChannelActivityFiveMinutes   int64 = 5 * 60
	ChannelActivityOneHour       int64 = 60 * 60
	ChannelActivityOneDay        int64 = 24 * 60 * 60
)

var channelActivityWindowSeconds = []int64{
	ChannelActivityWindowSeconds,
	ChannelActivityFiveMinutes,
	ChannelActivityOneHour,
	ChannelActivityOneDay,
}

type ChannelActivityDetail struct {
	Username      string `json:"username"`
	TokenName     string `json:"token_name"`
	Logs          int64  `json:"logs"`
	LastCreatedAt int64  `json:"last_created_at"`
}

type ChannelActivityWindow struct {
	WindowSeconds int64                   `json:"window_seconds"`
	Logs          int64                   `json:"logs"`
	Users         int64                   `json:"users"`
	Details       []ChannelActivityDetail `json:"details"`
}

type ChannelActivitySummary struct {
	ChannelId int                     `json:"channel_id"`
	Windows   []ChannelActivityWindow `json:"windows"`
}

// GetChannelActivitySummary uses the same logs table and time predicate as the
// usage-log page, so every window can be compared directly with log records.
func GetChannelActivitySummary(now int64) ([]ChannelActivitySummary, error) {
	if LOG_DB == nil {
		return []ChannelActivitySummary{}, nil
	}
	if now <= 0 {
		now = time.Now().Unix()
	}

	type logSummaryRow struct {
		ChannelId int    `gorm:"column:channel_id"`
		UserId    int    `gorm:"column:user_id"`
		Username  string `gorm:"column:username"`
		TokenName string `gorm:"column:token_name"`
		Logs60s   int64  `gorm:"column:logs_60s"`
		Last60s   int64  `gorm:"column:last_60s"`
		Logs5m    int64  `gorm:"column:logs_5m"`
		Last5m    int64  `gorm:"column:last_5m"`
		Logs1h    int64  `gorm:"column:logs_1h"`
		Last1h    int64  `gorm:"column:last_1h"`
		Logs24h   int64  `gorm:"column:logs_24h"`
		Last24h   int64  `gorm:"column:last_24h"`
	}
	type detailKey struct {
		userId    int
		username  string
		tokenName string
	}
	type windowAggregate struct {
		users   map[string]struct{}
		details map[detailKey]*ChannelActivityDetail
	}
	type channelAggregate struct {
		summary *ChannelActivitySummary
		windows []windowAggregate
	}

	cutoffs := []int64{
		now - ChannelActivityWindowSeconds,
		now - ChannelActivityFiveMinutes,
		now - ChannelActivityOneHour,
		now - ChannelActivityOneDay,
	}
	var rows []logSummaryRow
	if err := LOG_DB.Table("logs").
		Select(`channel_id, user_id, username, token_name,
SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END) AS logs_60s,
MAX(CASE WHEN created_at >= ? THEN created_at ELSE 0 END) AS last_60s,
SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END) AS logs_5m,
MAX(CASE WHEN created_at >= ? THEN created_at ELSE 0 END) AS last_5m,
SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END) AS logs_1h,
MAX(CASE WHEN created_at >= ? THEN created_at ELSE 0 END) AS last_1h,
COUNT(*) AS logs_24h,
MAX(created_at) AS last_24h`, cutoffs[0], cutoffs[0], cutoffs[1], cutoffs[1], cutoffs[2], cutoffs[2]).
		Where("channel_id > ? AND created_at >= ? AND created_at <= ?", 0, cutoffs[3], now).
		Group("channel_id, user_id, username, token_name").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	byChannel := make(map[int]*channelAggregate)
	for _, row := range rows {
		item := byChannel[row.ChannelId]
		if item == nil {
			item = &channelAggregate{
				summary: &ChannelActivitySummary{ChannelId: row.ChannelId},
				windows: make([]windowAggregate, len(channelActivityWindowSeconds)),
			}
			for index, seconds := range channelActivityWindowSeconds {
				item.summary.Windows = append(item.summary.Windows, ChannelActivityWindow{WindowSeconds: seconds})
				item.windows[index] = windowAggregate{
					users:   make(map[string]struct{}),
					details: make(map[detailKey]*ChannelActivityDetail),
				}
			}
			byChannel[row.ChannelId] = item
		}
		values := []struct {
			logs int64
			last int64
		}{
			{logs: row.Logs60s, last: row.Last60s},
			{logs: row.Logs5m, last: row.Last5m},
			{logs: row.Logs1h, last: row.Last1h},
			{logs: row.Logs24h, last: row.Last24h},
		}
		for index, value := range values {
			if value.logs <= 0 {
				continue
			}
			aggregate := &item.windows[index]
			userKey := fmt.Sprintf("%d", row.UserId)
			if row.UserId <= 0 {
				userKey = "name:" + row.Username
			}
			aggregate.users[userKey] = struct{}{}
			key := detailKey{userId: row.UserId, username: row.Username, tokenName: row.TokenName}
			aggregate.details[key] = &ChannelActivityDetail{
				Username:      row.Username,
				TokenName:     row.TokenName,
				Logs:          value.logs,
				LastCreatedAt: value.last,
			}
		}
	}

	items := make([]ChannelActivitySummary, 0, len(byChannel))
	for _, item := range byChannel {
		for index, aggregate := range item.windows {
			window := &item.summary.Windows[index]
			window.Users = int64(len(aggregate.users))
			window.Details = make([]ChannelActivityDetail, 0, len(aggregate.details))
			for _, detail := range aggregate.details {
				window.Logs += detail.Logs
				window.Details = append(window.Details, *detail)
			}
			sort.Slice(window.Details, func(i, j int) bool {
				left, right := window.Details[i], window.Details[j]
				if left.LastCreatedAt != right.LastCreatedAt {
					return left.LastCreatedAt > right.LastCreatedAt
				}
				if left.Username != right.Username {
					return left.Username < right.Username
				}
				return left.TokenName < right.TokenName
			})
		}
		items = append(items, *item.summary)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ChannelId < items[j].ChannelId })
	return items, nil
}
