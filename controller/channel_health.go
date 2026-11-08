package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const maxChannelHealthBatch = 500

// GetChannelsHealth returns the health state of the channels in the ids query
// (comma-separated). Without Redis it reports that health is unavailable.
func GetChannelsHealth(c *gin.Context) {
	var ids []int
	for part := range strings.SplitSeq(c.Query("ids"), ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil || id <= 0 {
			common.ApiErrorT(c, "Invalid channel ID")
			return
		}
		ids = append(ids, id)
	}
	if len(ids) > maxChannelHealthBatch {
		common.ApiErrorT(c, "Too many channel IDs")
		return
	}
	if common.RDB == nil {
		common.ApiSuccess(c, gin.H{"available": false, "items": []any{}})
		return
	}
	items, err := service.GetBatchChannelHealth(ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"available": true, "items": items})
}

// ResetChannelHealth clears a channel's warning and suspension state.
func ResetChannelHealth(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid channel ID")
		return
	}
	if err := service.ResetChannelHealth(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
