package ratio_setting

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

var defaultUpstreamWarningRatio = map[string]float64{}

var upstreamWarningRatioMap = types.NewRWMap[string, float64]()

var defaultUpstreamWarningAutoDisable = map[string]bool{}

var upstreamWarningAutoDisableMap = types.NewRWMap[string, bool]()

type GroupRatioSetting struct {
	GroupRatio                 *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio            *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupSpecialUsableGroup    *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
	UpstreamWarningRatio       *types.RWMap[string, float64]            `json:"upstream_warning_ratio"`
	UpstreamWarningAutoDisable *types.RWMap[string, bool]               `json:"upstream_warning_auto_disable"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)
	upstreamWarningRatioMap.AddAll(defaultUpstreamWarningRatio)
	upstreamWarningAutoDisableMap.AddAll(defaultUpstreamWarningAutoDisable)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup:    groupSpecialUsableGroup,
		GroupRatio:                 groupRatioMap,
		GroupGroupRatio:            groupGroupRatioMap,
		UpstreamWarningRatio:       upstreamWarningRatioMap,
		UpstreamWarningAutoDisable: upstreamWarningAutoDisableMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	if groupRatioSetting.UpstreamWarningRatio == nil {
		groupRatioSetting.UpstreamWarningRatio = types.NewRWMap[string, float64]()
		groupRatioSetting.UpstreamWarningRatio.AddAll(defaultUpstreamWarningRatio)
	}
	if groupRatioSetting.UpstreamWarningAutoDisable == nil {
		groupRatioSetting.UpstreamWarningAutoDisable = types.NewRWMap[string, bool]()
		groupRatioSetting.UpstreamWarningAutoDisable.AddAll(defaultUpstreamWarningAutoDisable)
	}
	return &groupRatioSetting
}

func GetUpstreamWarningSettings() (map[string]float64, map[string]bool) {
	return upstreamWarningRatioMap.ReadAll(), upstreamWarningAutoDisableMap.ReadAll()
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := common.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}

func CheckUpstreamWarningRatio(jsonStr string) error {
	checkRatios := make(map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &checkRatios); err != nil {
		return err
	}
	for name, ratio := range checkRatios {
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
			return errors.New("upstream warning ratio must be a finite non-negative number: " + name)
		}
	}
	return nil
}

func CheckUpstreamWarningAutoDisable(jsonStr string) error {
	checkSwitches := make(map[string]bool)
	return common.Unmarshal([]byte(jsonStr), &checkSwitches)
}
