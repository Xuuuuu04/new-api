package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type MappingRow struct {
	GroupName     string  `gorm:"column:group_name"`
	Model         string  `gorm:"column:model"`
	ChannelId     int     `gorm:"column:channel_id"`
	Enabled       bool    `gorm:"column:enabled"`
	Priority      *int64  `gorm:"column:priority"`
	Weight        uint    `gorm:"column:weight"`
	Tag           *string `gorm:"column:tag"`
	ChannelName   string  `gorm:"column:channel_name"`
	ChannelType   int     `gorm:"column:channel_type"`
	ChannelStatus int     `gorm:"column:channel_status"`
	BaseURL       *string `gorm:"column:base_url"`
	ModelMapping  *string `gorm:"column:model_mapping"`
}

func GetMappingRows(group string, enabledOnly bool, keyword string) ([]MappingRow, error) {
	var rows []MappingRow
	groupCol := fmt.Sprintf("abilities.%s", commonGroupCol)
	selectCols := fmt.Sprintf("%s as group_name, abilities.model, abilities.channel_id, abilities.enabled, abilities.priority, abilities.weight, abilities.tag, channels.name as channel_name, channels.type as channel_type, channels.status as channel_status, channels.base_url, channels.model_mapping", groupCol)

	query := DB.Table("abilities").
		Select(selectCols).
		Joins("left join channels on abilities.channel_id = channels.id")

	if group != "" {
		query = query.Where(groupCol+" = ?", group)
	}
	if enabledOnly {
		query = query.Where("abilities.enabled = ?", true).
			Where("channels.status = ?", common.ChannelStatusEnabled)
	}
	if keyword != "" {
		query = query.Where("abilities.model LIKE ?", "%"+strings.TrimSpace(keyword)+"%")
	}

	order := fmt.Sprintf("%s asc, abilities.model asc, abilities.priority desc, abilities.weight desc", groupCol)
	err := query.Order(order).Scan(&rows).Error
	return rows, err
}

func UpdateAbility(group string, model string, channelId int, priority *int64, weight *uint) error {
	updates := map[string]interface{}{}
	if priority != nil {
		updates["priority"] = *priority
	}
	if weight != nil {
		updates["weight"] = *weight
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ?", group, model, channelId).
		Updates(updates).Error
}

func UpdateAbilityEnabled(group string, model string, channelId int, enabled bool) error {
	return DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ?", group, model, channelId).
		Update("enabled", enabled).Error
}

func UpdateChannelModelMapping(channelId int, mapping *string) error {
	return DB.Model(&Channel{}).
		Where("id = ?", channelId).
		Update("model_mapping", mapping).Error
}
