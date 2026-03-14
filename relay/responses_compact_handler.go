package relay

import (
	"fmt"
	"net/http"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// ResponsesCompactHelper handles the /v1/responses/compact endpoint.
// This is a stub that will be replaced with the full implementation in Task 19.
func ResponsesCompactHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	return types.NewErrorWithStatusCode(
		fmt.Errorf("responses compact endpoint not yet implemented"),
		types.ErrorCodeInvalidRequest,
		http.StatusNotImplemented,
		types.ErrOptionWithSkipRetry(),
	)
}
