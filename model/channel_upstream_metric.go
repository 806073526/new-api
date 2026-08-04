package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"gorm.io/gorm"
)

// UpstreamKeyFingerprint returns a stable, non-reversible identifier for a channel key.
// It intentionally normalizes the formats accepted by new-api clients so a raw key and
// its common Bearer/sk- representations resolve to the same upstream account key.
func UpstreamKeyFingerprint(key string) string {
	key = strings.TrimSpace(key)
	if len(key) >= 7 && strings.EqualFold(key[:7], "bearer ") {
		key = strings.TrimSpace(key[7:])
	}
	key = strings.TrimPrefix(key, "sk-")
	if strings.TrimSpace(key) == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// ChannelUpstreamMetric is the latest upstream-hub observation for one new-api
// channel. It is deliberately separate from Channel so upstream failures never
// change channel status or routing abilities.
type ChannelUpstreamMetric struct {
	ID                 int       `json:"id" gorm:"primaryKey"`
	ChannelId          int       `json:"channel_id" gorm:"not null;uniqueIndex"`
	UpstreamChannelId  int       `json:"upstream_channel_id"`
	UpstreamGroup      string    `json:"upstream_group" gorm:"size:128"`
	UpstreamRatio      float64   `json:"upstream_ratio"`
	UpstreamBalance    *float64  `json:"upstream_balance,omitempty"`
	RatioUpdatedTime   int64     `json:"ratio_updated_time"`
	BalanceUpdatedTime int64     `json:"balance_updated_time"`
	SyncStatus         string    `json:"sync_status" gorm:"size:32"`
	SyncError          string    `json:"sync_error,omitempty" gorm:"size:512"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (ChannelUpstreamMetric) TableName() string { return "channel_upstream_metrics" }

func UpsertChannelUpstreamMetric(metric *ChannelUpstreamMetric) error {
	var current ChannelUpstreamMetric
	err := DB.Where("channel_id = ?", metric.ChannelId).First(&current).Error
	if err == gorm.ErrRecordNotFound {
		return DB.Create(metric).Error
	}
	if err != nil {
		return err
	}
	metric.ID = current.ID
	return DB.Model(&current).Updates(map[string]any{
		"channel_id":           metric.ChannelId,
		"upstream_channel_id":  metric.UpstreamChannelId,
		"upstream_group":       metric.UpstreamGroup,
		"upstream_ratio":       metric.UpstreamRatio,
		"upstream_balance":     metric.UpstreamBalance,
		"ratio_updated_time":   metric.RatioUpdatedTime,
		"balance_updated_time": metric.BalanceUpdatedTime,
		"sync_status":          metric.SyncStatus,
		"sync_error":           metric.SyncError,
		"updated_at":           time.Now(),
	}).Error
}

func DeleteChannelUpstreamMetric(channelID int) error {
	err := DB.Where("channel_id = ?", channelID).Delete(&ChannelUpstreamMetric{}).Error
	if err != nil && (strings.Contains(strings.ToLower(err.Error()), "no such table") || strings.Contains(strings.ToLower(err.Error()), "does not exist")) {
		return nil
	}
	return err
}

// EnrichChannelsWithUpstreamMetrics copies the latest snapshot into transient
// JSON fields on channels. It performs one query for the whole page.
func EnrichChannelsWithUpstreamMetrics(channels []*Channel) error {
	if len(channels) == 0 {
		return nil
	}
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel != nil {
			ids = append(ids, channel.Id)
		}
	}
	var metrics []ChannelUpstreamMetric
	if err := DB.Where("channel_id IN ?", ids).Find(&metrics).Error; err != nil {
		return err
	}
	byChannel := make(map[int]ChannelUpstreamMetric, len(metrics))
	for _, metric := range metrics {
		byChannel[metric.ChannelId] = metric
	}
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if metric, ok := byChannel[channel.Id]; ok {
			channel.UpstreamGroup = metric.UpstreamGroup
			channel.UpstreamRatio = metric.UpstreamRatio
			channel.UpstreamBalance = metric.UpstreamBalance
			channel.UpstreamRatioUpdatedTime = metric.RatioUpdatedTime
			channel.UpstreamBalanceUpdatedTime = metric.BalanceUpdatedTime
			channel.UpstreamSyncStatus = metric.SyncStatus
			channel.UpstreamSyncError = metric.SyncError
		}
	}
	return nil
}
