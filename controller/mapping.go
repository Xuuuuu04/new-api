package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type MappingChannelItem struct {
	ChannelId      int    `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	ChannelType    int    `json:"channel_type"`
	ChannelStatus  int    `json:"channel_status"`
	AbilityEnabled bool   `json:"ability_enabled"`
	Weight         uint   `json:"weight"`
	Priority       int64  `json:"priority"`
	Tag            string `json:"tag"`
	BaseURL        string `json:"base_url"`
	UpstreamModel  string `json:"upstream_model"`
	MappingApplied bool   `json:"mapping_applied"`
}

type MappingItem struct {
	Group    string               `json:"group"`
	Model    string               `json:"model"`
	Channels []MappingChannelItem `json:"channels"`
}

type MappingResponse struct {
	Groups []string      `json:"groups"`
	Items  []MappingItem `json:"items"`
}

func GetMappings(c *gin.Context) {
	group := strings.TrimSpace(c.Query("group"))
	keyword := strings.TrimSpace(c.Query("q"))
	enabledOnly := true
	enabledQuery := strings.TrimSpace(c.Query("enabled_only"))
	if enabledQuery != "" && (enabledQuery == "false" || enabledQuery == "0") {
		enabledOnly = false
	}

	rows, err := model.GetMappingRows(group, enabledOnly, keyword)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	groupsSet := make(map[string]struct{})
	itemMap := make(map[string]*MappingItem)
	channelMappingCache := make(map[int]map[string]string)

	for _, row := range rows {
		groupName := row.GroupName
		groupsSet[groupName] = struct{}{}

		key := groupName + "||" + row.Model
		item, ok := itemMap[key]
		if !ok {
			item = &MappingItem{
				Group:    groupName,
				Model:    row.Model,
				Channels: make([]MappingChannelItem, 0),
			}
			itemMap[key] = item
		}

		mapping, ok := channelMappingCache[row.ChannelId]
		if !ok {
			mapping = map[string]string{}
			if row.ModelMapping != nil && *row.ModelMapping != "" && *row.ModelMapping != "{}" {
				_ = json.Unmarshal([]byte(*row.ModelMapping), &mapping)
			}
			channelMappingCache[row.ChannelId] = mapping
		}

		upstreamModel := row.Model
		mappingApplied := false
		if mapped, exists := mapping[row.Model]; exists && mapped != "" {
			upstreamModel = mapped
			mappingApplied = true
		}

		baseURL := ""
		if row.BaseURL != nil {
			baseURL = *row.BaseURL
		}
		if baseURL == "" && row.ChannelType >= 0 && row.ChannelType < len(constant.ChannelBaseURLs) {
			baseURL = constant.ChannelBaseURLs[row.ChannelType]
		}

		priority := int64(0)
		if row.Priority != nil {
			priority = *row.Priority
		}

		tag := ""
		if row.Tag != nil {
			tag = *row.Tag
		}

		item.Channels = append(item.Channels, MappingChannelItem{
			ChannelId:      row.ChannelId,
			ChannelName:    row.ChannelName,
			ChannelType:    row.ChannelType,
			ChannelStatus:  row.ChannelStatus,
			AbilityEnabled: row.Enabled,
			Weight:         row.Weight,
			Priority:       priority,
			Tag:            tag,
			BaseURL:        baseURL,
			UpstreamModel:  upstreamModel,
			MappingApplied: mappingApplied,
		})
	}

	groups := make([]string, 0, len(groupsSet))
	for g := range groupsSet {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	items := make([]MappingItem, 0, len(itemMap))
	for _, item := range itemMap {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Group == items[j].Group {
			return items[i].Model < items[j].Model
		}
		return items[i].Group < items[j].Group
	})

	common.ApiSuccess(c, MappingResponse{
		Groups: groups,
		Items:  items,
	})
}

type UpdateAbilityRequest struct {
	Group     string `json:"group"`
	Model     string `json:"model"`
	ChannelId int    `json:"channel_id"`
	Weight    *uint  `json:"weight"`
	Priority  *int64 `json:"priority"`
}

func UpdateMappingAbility(c *gin.Context) {
	var req UpdateAbilityRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Group == "" || req.Model == "" || req.ChannelId == 0 {
		common.ApiError(c, errors.New("invalid params"))
		return
	}
	if req.Weight == nil && req.Priority == nil {
		common.ApiError(c, errors.New("no updates"))
		return
	}
	if err := model.UpdateAbility(req.Group, req.Model, req.ChannelId, req.Priority, req.Weight); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

type UpdateAbilityEnabledRequest struct {
	Group     string `json:"group"`
	Model     string `json:"model"`
	ChannelId int    `json:"channel_id"`
	Enabled   *bool  `json:"enabled"`
}

func UpdateMappingAbilityEnabled(c *gin.Context) {
	var req UpdateAbilityEnabledRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Group == "" || req.Model == "" || req.ChannelId == 0 || req.Enabled == nil {
		common.ApiError(c, errors.New("invalid params"))
		return
	}
	if err := model.UpdateAbilityEnabled(req.Group, req.Model, req.ChannelId, *req.Enabled); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

type MappingPair struct {
	Model         string `json:"model"`
	UpstreamModel string `json:"upstream_model"`
}

type UpdateChannelMappingRequest struct {
	ChannelId int           `json:"channel_id"`
	Mappings  []MappingPair `json:"mappings"`
}

func UpdateChannelMapping(c *gin.Context) {
	var req UpdateChannelMappingRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ChannelId == 0 {
		common.ApiError(c, errors.New("invalid channel_id"))
		return
	}

	mapping := make(map[string]string)
	for _, pair := range req.Mappings {
		modelName := strings.TrimSpace(pair.Model)
		upstreamName := strings.TrimSpace(pair.UpstreamModel)
		if modelName == "" || upstreamName == "" {
			continue
		}
		if _, exists := mapping[modelName]; exists {
			common.ApiError(c, errors.New("duplicate model mapping"))
			return
		}
		mapping[modelName] = upstreamName
	}

	var mappingPtr *string
	if len(mapping) > 0 {
		bytes, _ := json.Marshal(mapping)
		mappingStr := string(bytes)
		mappingPtr = common.GetPointer(mappingStr)
	}

	if err := model.UpdateChannelModelMapping(req.ChannelId, mappingPtr); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

type UpdateChannelStatusRequest struct {
	ChannelId int   `json:"channel_id"`
	Enabled   *bool `json:"enabled"`
}

func UpdateMappingChannelStatus(c *gin.Context) {
	var req UpdateChannelStatusRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ChannelId == 0 || req.Enabled == nil {
		common.ApiError(c, errors.New("invalid params"))
		return
	}

	status := common.ChannelStatusManuallyDisabled
	if *req.Enabled {
		status = common.ChannelStatusEnabled
	}
	updated := model.UpdateChannelStatusManual(req.ChannelId, status, "manual toggle from mapping")
	if !updated {
		common.ApiError(c, errors.New("failed to update channel status"))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}
