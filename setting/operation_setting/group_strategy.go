package operation_setting

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	DefaultGroupStrategyTimeoutHTTPStatus   = 503
	DefaultGroupStrategyTimeoutErrorMessage = "资源繁忙，请稍后尝试"
)

type GroupStrategy struct {
	Enabled                           bool   `json:"enabled"`
	StreamRetryFirstByteBudgetSeconds *int   `json:"stream_retry_first_byte_budget_seconds,omitempty"`
	RetryTimes                        *int   `json:"retry_times,omitempty"`
	TimeoutHTTPStatus                 *int   `json:"timeout_http_status,omitempty"`
	TimeoutErrorMessage               string `json:"timeout_error_message,omitempty"`
}

type GroupStrategySettings struct {
	mu    sync.RWMutex
	items map[string]GroupStrategy
}

var groupStrategySettings = &GroupStrategySettings{
	items: make(map[string]GroupStrategy),
}

func init() {
	config.GlobalConfig.Register("group_strategy_settings", groupStrategySettings)
}

func (s *GroupStrategySettings) MarshalJSON() ([]byte, error) {
	return common.Marshal(s.ReadAll())
}

func (s *GroupStrategySettings) UnmarshalJSON(data []byte) error {
	parsed := make(map[string]GroupStrategy)
	trimmed := strings.TrimSpace(string(data))
	if trimmed != "" && trimmed != "null" {
		if err := common.Unmarshal(data, &parsed); err != nil {
			return err
		}
	}
	if err := validateGroupStrategyMap(parsed); err != nil {
		return err
	}
	s.WriteAll(parsed)
	return nil
}

func (s *GroupStrategySettings) ReadAll() map[string]GroupStrategy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copied := make(map[string]GroupStrategy, len(s.items))
	for k, v := range s.items {
		copied[k] = cloneGroupStrategy(v)
	}
	return copied
}

func (s *GroupStrategySettings) WriteAll(items map[string]GroupStrategy) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items = make(map[string]GroupStrategy, len(items))
	for k, v := range items {
		s.items[k] = cloneGroupStrategy(v)
	}
}

func cloneGroupStrategy(strategy GroupStrategy) GroupStrategy {
	cloned := strategy
	if strategy.StreamRetryFirstByteBudgetSeconds != nil {
		value := *strategy.StreamRetryFirstByteBudgetSeconds
		cloned.StreamRetryFirstByteBudgetSeconds = &value
	}
	if strategy.RetryTimes != nil {
		value := *strategy.RetryTimes
		cloned.RetryTimes = &value
	}
	if strategy.TimeoutHTTPStatus != nil {
		value := *strategy.TimeoutHTTPStatus
		cloned.TimeoutHTTPStatus = &value
	}
	return cloned
}

func ValidateGroupStrategySettings(raw string) error {
	settings := make(map[string]GroupStrategy)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := common.UnmarshalJsonStr(raw, &settings); err != nil {
		return err
	}
	return validateGroupStrategyMap(settings)
}

func UpdateGroupStrategySettingsByJSONString(raw string) error {
	if strings.TrimSpace(raw) == "" {
		groupStrategySettings.WriteAll(make(map[string]GroupStrategy))
		return nil
	}
	settings := make(map[string]GroupStrategy)
	if err := common.UnmarshalJsonStr(raw, &settings); err != nil {
		return err
	}
	if err := validateGroupStrategyMap(settings); err != nil {
		return err
	}
	groupStrategySettings.WriteAll(settings)
	return nil
}

func GetGroupStrategySettings() *GroupStrategySettings {
	return groupStrategySettings
}

func GetGroupStrategySettingsCopy() map[string]GroupStrategy {
	return groupStrategySettings.ReadAll()
}

func ResolveGroupStrategy(group string) (GroupStrategy, bool) {
	if strings.TrimSpace(group) == "" {
		return GroupStrategy{}, false
	}
	items := groupStrategySettings.ReadAll()
	strategy, ok := items[group]
	if !ok || !strategy.Enabled {
		return GroupStrategy{}, false
	}
	return strategy, true
}

func validateGroupStrategyMap(settings map[string]GroupStrategy) error {
	for group, strategy := range settings {
		group = strings.TrimSpace(group)
		if group == "" {
			return errors.New("group strategy key must not be empty")
		}
		if !groupExistsInRatioOption(group) {
			return fmt.Errorf("group strategy group not found in GroupRatio: %s", group)
		}
		if strategy.StreamRetryFirstByteBudgetSeconds != nil && *strategy.StreamRetryFirstByteBudgetSeconds <= 0 {
			return fmt.Errorf("group strategy %s stream_retry_first_byte_budget_seconds must be greater than 0", group)
		}
		if strategy.RetryTimes != nil && *strategy.RetryTimes < 0 {
			return fmt.Errorf("group strategy %s retry_times must be greater than or equal to 0", group)
		}
		if strategy.TimeoutHTTPStatus != nil {
			status := *strategy.TimeoutHTTPStatus
			if status < 100 || status > 599 {
				return fmt.Errorf("group strategy %s timeout_http_status must be between 100 and 599", group)
			}
		}
	}
	return nil
}

func groupExistsInRatioOption(group string) bool {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["GroupRatio"]
	common.OptionMapRWMutex.RUnlock()

	if strings.TrimSpace(raw) == "" {
		return false
	}

	groups := make(map[string]float64)
	if err := common.UnmarshalJsonStr(raw, &groups); err != nil {
		return false
	}
	_, ok := groups[group]
	return ok
}
