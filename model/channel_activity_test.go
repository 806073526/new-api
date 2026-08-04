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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelActivitySummaryMatchesRecentLogRecords(t *testing.T) {
	truncateTables(t)

	now := int64(1000)
	logs := []Log{
		{
			ChannelId: 7, UserId: 101, Username: "alice", TokenName: "team-key",
			CreatedAt: now - 1, Type: LogTypeConsume,
		},
		{
			ChannelId: 7, UserId: 101, Username: "alice", TokenName: "team-key",
			CreatedAt: now - 2, Type: LogTypeError,
		},
		{
			ChannelId: 7, UserId: 202, Username: "bob", TokenName: "default",
			CreatedAt: now - 61, Type: LogTypeConsume,
		},
		{
			ChannelId: 7, UserId: 303, Username: "carol", TokenName: "backup",
			CreatedAt: now - 301, Type: LogTypeConsume,
		},
		{
			ChannelId: 7, UserId: 404, Username: "dave", TokenName: "long-term",
			CreatedAt: now - 3601, Type: LogTypeError,
		},
		{
			ChannelId: 7, UserId: 505, Username: "old-user", TokenName: "old-token",
			CreatedAt: now - 86401, Type: LogTypeConsume,
		},
		{
			ChannelId: 0, UserId: 606, Username: "admin",
			CreatedAt: now - 1, Type: LogTypeManage,
		},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	summary, err := GetChannelActivitySummary(now)

	require.NoError(t, err)
	require.Len(t, summary, 1)
	assert.Equal(t, 7, summary[0].ChannelId)
	require.Len(t, summary[0].Windows, 4)

	assert.Equal(t, int64(2), summary[0].Windows[0].Logs)
	assert.Equal(t, int64(1), summary[0].Windows[0].Users)
	require.Len(t, summary[0].Windows[0].Details, 1)
	assert.Equal(t, int64(2), summary[0].Windows[0].Details[0].Logs)

	assert.Equal(t, int64(3), summary[0].Windows[1].Logs)
	assert.Equal(t, int64(2), summary[0].Windows[1].Users)
	assert.Equal(t, int64(4), summary[0].Windows[2].Logs)
	assert.Equal(t, int64(3), summary[0].Windows[2].Users)
	assert.Equal(t, int64(5), summary[0].Windows[3].Logs)
	assert.Equal(t, int64(4), summary[0].Windows[3].Users)
}
