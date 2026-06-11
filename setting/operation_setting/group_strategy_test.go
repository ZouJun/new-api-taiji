package operation_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestGroupStrategyValidateGroupStrategySettings(t *testing.T) {
	prepareGroupStrategyTestOptions()
	valid := `{"default":{"enabled":true,"stream_retry_first_byte_budget_seconds":10,"retry_times":2,"timeout_http_status":503,"timeout_error_message":"busy"}}`
	if err := ValidateGroupStrategySettings(valid); err != nil {
		t.Fatalf("expected valid settings, got %v", err)
	}
}

func TestGroupStrategyValidateRejectsInvalidStatus(t *testing.T) {
	prepareGroupStrategyTestOptions()
	invalid := `{"default":{"enabled":true,"timeout_http_status":99}}`
	if err := ValidateGroupStrategySettings(invalid); err == nil {
		t.Fatal("expected invalid timeout_http_status to be rejected")
	}
}

func TestGroupStrategyValidateRejectsNegativeValues(t *testing.T) {
	prepareGroupStrategyTestOptions()
	invalidBudget := `{"default":{"enabled":true,"stream_retry_first_byte_budget_seconds":0}}`
	if err := ValidateGroupStrategySettings(invalidBudget); err == nil {
		t.Fatal("expected invalid stream retry first byte budget to be rejected")
	}

	invalidRetry := `{"default":{"enabled":true,"retry_times":-1}}`
	if err := ValidateGroupStrategySettings(invalidRetry); err == nil {
		t.Fatal("expected invalid retry_times to be rejected")
	}
}

func TestGroupStrategyResolveGroupStrategyDisabledAndMissingFallback(t *testing.T) {
	prepareGroupStrategyTestOptions()
	original := GetGroupStrategySettingsCopy()
	defer groupStrategySettings.WriteAll(original)

	if err := UpdateGroupStrategySettingsByJSONString(`{"default":{"enabled":false,"retry_times":2}}`); err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}
	if _, ok := ResolveGroupStrategy("default"); ok {
		t.Fatal("expected disabled strategy to fall back")
	}
	if _, ok := ResolveGroupStrategy("vip"); ok {
		t.Fatal("expected missing strategy to fall back")
	}
}

func prepareGroupStrategyTestOptions() {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap["GroupRatio"] = `{"default":1,"vip":1}`
}
