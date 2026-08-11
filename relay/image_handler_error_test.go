package relay

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/types"
)

func TestNewImageConversionErrorPropagatesHealthExemption(t *testing.T) {
	sourceErr := types.NewNoRecordChannelHealthError(errors.New("source download timeout"))
	apiErr := newImageConversionError(sourceErr)
	if types.IsRecordChannelHealth(apiErr) {
		t.Fatal("source preparation error must retain the channel-health exemption")
	}

	channelErr := newImageConversionError(errors.New("replicate upload failed"))
	if !types.IsRecordChannelHealth(channelErr) {
		t.Fatal("ordinary conversion errors must remain eligible for health recording")
	}
}
