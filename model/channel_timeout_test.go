package model

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestChannelValidateSettingsRejectsInvalidTimeouts(t *testing.T) {
	zero := 0
	channel := &Channel{}
	channel.SetSetting(dto.ChannelSettings{
		NonStreamTimeoutSeconds: &zero,
	})
	if err := channel.ValidateSettings(); err == nil {
		t.Fatal("expected invalid non-stream timeout to be rejected")
	}

	channel.SetSetting(dto.ChannelSettings{
		StreamFirstByteTimeoutSeconds: &zero,
	})
	if err := channel.ValidateSettings(); err == nil {
		t.Fatal("expected invalid stream first-byte timeout to be rejected")
	}

	channel.SetSetting(dto.ChannelSettings{
		AwsInvokeTimeoutSeconds: &zero,
	})
	if err := channel.ValidateSettings(); err == nil {
		t.Fatal("expected invalid aws invoke timeout to be rejected")
	}

	channel.SetSetting(dto.ChannelSettings{
		AwsSDKMaxAttempts: &zero,
	})
	if err := channel.ValidateSettings(); err == nil {
		t.Fatal("expected invalid aws sdk max attempts to be rejected")
	}
}
