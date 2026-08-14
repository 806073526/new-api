package model

import (
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

// ChannelModelManualDisable is an operator-controlled routing override. Unlike
// channel model health, it never expires or participates in automatic probes.
type ChannelModelManualDisable struct {
	Id              int64  `json:"id" gorm:"primaryKey"`
	ChannelId       int    `json:"channel_id" gorm:"uniqueIndex:uniq_channel_model_manual_disable_key,priority:1"`
	Model           string `json:"model" gorm:"type:varchar(255);uniqueIndex:uniq_channel_model_manual_disable_key,priority:2"`
	Active          bool   `json:"active" gorm:"index"`
	Reason          string `json:"reason" gorm:"type:text"`
	OperatorId      int    `json:"operator_id"`
	OperatorName    string `json:"operator_name" gorm:"type:varchar(255)"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt       int64  `json:"updated_at" gorm:"bigint;index"`
	RecoveredAt     int64  `json:"recovered_at" gorm:"bigint"`
	RecoveredBy     int    `json:"recovered_by"`
	RecoveredByName string `json:"recovered_by_name" gorm:"type:varchar(255)"`
}

type channelModelManualDisableView struct {
	ChannelId    int    `gorm:"column:channel_id"`
	Model        string `gorm:"column:model"`
	Reason       string `gorm:"column:reason"`
	OperatorId   int    `gorm:"column:operator_id"`
	OperatorName string `gorm:"column:operator_name"`
	CreatedAt    int64  `gorm:"column:created_at"`
}

func (ChannelModelManualDisable) TableName() string {
	return "channel_model_manual_disables"
}

type channelModelManualDisableKey struct {
	ChannelId int
	Model     string
}

var channelModelManualDisableCache = struct {
	sync.RWMutex
	items map[channelModelManualDisableKey]*ChannelModelManualDisable
}{items: make(map[channelModelManualDisableKey]*ChannelModelManualDisable)}

func normalizeChannelModelManualDisableKey(channelId int, model string) channelModelManualDisableKey {
	return channelModelManualDisableKey{ChannelId: channelId, Model: strings.TrimSpace(model)}
}

func InitChannelModelManualDisableCache() {
	if DB == nil {
		return
	}
	var items []*ChannelModelManualDisable
	if err := DB.Where("active = ?", true).Find(&items).Error; err != nil {
		return
	}
	cache := make(map[channelModelManualDisableKey]*ChannelModelManualDisable, len(items))
	for _, item := range items {
		cache[normalizeChannelModelManualDisableKey(item.ChannelId, item.Model)] = item
	}
	channelModelManualDisableCache.Lock()
	channelModelManualDisableCache.items = cache
	channelModelManualDisableCache.Unlock()
}

func IsChannelModelManuallyDisabled(channelId int, model string) bool {
	if !common.MemoryCacheEnabled {
		if DB == nil {
			return false
		}
		var count int64
		if err := DB.Model(&ChannelModelManualDisable{}).
			Where("channel_id = ? AND model = ? AND active = ?", channelId, strings.TrimSpace(model), true).
			Count(&count).Error; err != nil {
			return false
		}
		return count > 0
	}
	key := normalizeChannelModelManualDisableKey(channelId, model)
	channelModelManualDisableCache.RLock()
	_, disabled := channelModelManualDisableCache.items[key]
	channelModelManualDisableCache.RUnlock()
	return disabled
}

func filterChannelModelManualDisableCandidates(channelIds []int, group, model string) []int {
	if len(channelIds) == 0 {
		return channelIds
	}
	if !common.MemoryCacheEnabled && DB != nil {
		var disabledChannelIds []int
		if err := DB.Model(&ChannelModelManualDisable{}).
			Where("channel_id IN ? AND model = ? AND active = ?", channelIds, strings.TrimSpace(model), true).
			Pluck("channel_id", &disabledChannelIds).Error; err == nil {
			disabled := make(map[int]struct{}, len(disabledChannelIds))
			for _, channelId := range disabledChannelIds {
				disabled[channelId] = struct{}{}
			}
			filtered := make([]int, 0, len(channelIds))
			for _, channelId := range channelIds {
				if _, exists := disabled[channelId]; !exists {
					filtered = append(filtered, channelId)
				}
			}
			return filtered
		}
	}
	filtered := make([]int, 0, len(channelIds))
	for _, channelId := range channelIds {
		if !IsChannelModelManuallyDisabled(channelId, model) {
			filtered = append(filtered, channelId)
		}
	}
	return filtered
}

func activeChannelModelManualDisables() (map[channelModelManualDisableKey]channelModelManualDisableView, error) {
	items := make(map[channelModelManualDisableKey]channelModelManualDisableView)
	if DB == nil {
		return items, nil
	}
	var rows []channelModelManualDisableView
	if err := DB.Model(&ChannelModelManualDisable{}).
		Select("channel_id, model, reason, operator_id, operator_name, created_at").
		Where("active = ?", true).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		items[normalizeChannelModelManualDisableKey(row.ChannelId, row.Model)] = row
	}
	return items, nil
}

func DisableChannelModelManually(channelId int, model, reason string, operatorId int, operatorName string, now int64) error {
	model = strings.TrimSpace(model)
	if channelId <= 0 || model == "" {
		return fmt.Errorf("invalid channel model manual disable request")
	}
	if DB == nil {
		return fmt.Errorf("database is not initialized")
	}
	if now <= 0 {
		now = common.GetTimestamp()
	}
	item := &ChannelModelManualDisable{
		ChannelId:    channelId,
		Model:        model,
		Active:       true,
		Reason:       strings.TrimSpace(reason),
		OperatorId:   operatorId,
		OperatorName: strings.TrimSpace(operatorName),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	updates := map[string]interface{}{
		"active":            true,
		"reason":            item.Reason,
		"operator_id":       operatorId,
		"operator_name":     item.OperatorName,
		"created_at":        now,
		"updated_at":        now,
		"recovered_at":      int64(0),
		"recovered_by":      0,
		"recovered_by_name": "",
	}
	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "model"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(item).Error; err != nil {
		return err
	}
	key := normalizeChannelModelManualDisableKey(channelId, model)
	channelModelManualDisableCache.Lock()
	channelModelManualDisableCache.items[key] = item
	channelModelManualDisableCache.Unlock()
	return nil
}

func RecoverChannelModelManuallyDisabled(channelId int, model string, operatorId int, operatorName string, now int64) error {
	model = strings.TrimSpace(model)
	if channelId <= 0 || model == "" {
		return fmt.Errorf("invalid channel model manual recovery request")
	}
	if DB == nil {
		return fmt.Errorf("database is not initialized")
	}
	if now <= 0 {
		now = common.GetTimestamp()
	}
	result := DB.Model(&ChannelModelManualDisable{}).
		Where("channel_id = ? AND model = ? AND active = ?", channelId, model, true).
		Updates(map[string]interface{}{
			"active":            false,
			"updated_at":        now,
			"recovered_at":      now,
			"recovered_by":      operatorId,
			"recovered_by_name": strings.TrimSpace(operatorName),
		})
	if result.Error != nil {
		return result.Error
	}
	key := normalizeChannelModelManualDisableKey(channelId, model)
	channelModelManualDisableCache.Lock()
	delete(channelModelManualDisableCache.items, key)
	channelModelManualDisableCache.Unlock()
	return ResetChannelModelHealth(channelId, "", model)
}

func DeleteChannelModelManualDisables(channelIds []int) error {
	if DB == nil || len(channelIds) == 0 {
		return nil
	}
	if err := DB.Where("channel_id IN ?", channelIds).Delete(&ChannelModelManualDisable{}).Error; err != nil {
		return err
	}
	channelModelManualDisableCache.Lock()
	for key := range channelModelManualDisableCache.items {
		for _, channelId := range channelIds {
			if key.ChannelId == channelId {
				delete(channelModelManualDisableCache.items, key)
				break
			}
		}
	}
	channelModelManualDisableCache.Unlock()
	return nil
}
