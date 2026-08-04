package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// UpstreamHubAuth protects the machine-to-machine sync endpoints with a token
// that is independent from dashboard sessions. Configure the same value in
// UPSTREAM_HUB_API_TOKEN on new-api and upstream-hub.
func UpstreamHubAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(common.GetEnvOrDefaultString("UPSTREAM_HUB_API_TOKEN", ""))
		raw := strings.TrimSpace(c.GetHeader("Authorization"))
		provided := ""
		if len(raw) >= 7 && strings.EqualFold(raw[:7], "bearer ") {
			provided = strings.TrimSpace(raw[7:])
		}
		if expected == "" || provided == "" || provided != expected {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "upstream-hub authentication required"})
			return
		}
		c.Next()
	}
}
