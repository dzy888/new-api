package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// CanvasSessionOnly excludes PATs and API keys. UserAuth validates each live login session first.
func CanvasSessionOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := GetSessionAuthIdentity(c); !ok || c.GetBool("use_access_token") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_SESSION_REQUIRED", "message": "A login session is required"})
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Next()
	}
}

// CanvasRelayAuth gives the existing relay a user-funded context, without creating an API key.
func CanvasRelayAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := GetSessionAuthIdentity(c); !ok || c.GetBool("use_access_token") {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, "/api/canvas/relay")
		c.Request.URL.RawPath = ""
		c.Request.RequestURI = c.Request.URL.RequestURI()
		user, err := model.GetUserCache(c.GetInt("id"))
		if err != nil {
			common.ApiError(c, err)
			c.Abort()
			return
		}
		user.WriteContext(c)
		group := user.Group
		if c.Request.Method == http.MethodPost {
			request, _, err := getModelRequest(c)
			if err != nil || request == nil || request.Model == "" {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "A valid model is required"}})
				return
			}
			offers := service.GetCanvasModels(user.Group)
			index := slices.IndexFunc(offers, func(offer service.CanvasModel) bool { return offer.Name == request.Model })
			if index < 0 {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "This model is unavailable for your account"}})
				return
			}
			// Any client-supplied group is ignored; both catalog and relay resolve the same offer.
			group = offers[index].Group
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
		if err := SetupContextForToken(c, &model.Token{UserId: user.Id, Name: "DmxVideo", Group: group, UnlimitedQuota: true}); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Set("canvas_session", true)
		// Even an administrator using the canvas can only retrieve their own tasks and artifacts.
		c.Set("role", common.RoleCommonUser)
		for _, header := range []string{"Authorization", "Cookie", "X-Auth-Session", "X-Api-Key", "X-Goog-Api-Key"} {
			c.Request.Header.Del(header)
		}
		c.Next()
	}
}
