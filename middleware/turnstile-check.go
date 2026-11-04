package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type turnstileCheckResponse struct {
	Success bool `json:"success"`
}

var turnstileClient = &http.Client{Timeout: 10 * time.Second}

// A required check is used by password login even when the global optional
// Turnstile switch for other operations is off.
func TurnstileCheck(required ...bool) gin.HandlerFunc {
	force := len(required) > 0 && required[0]
	return func(c *gin.Context) {
		if !common.TurnstileCheckEnabled && !force {
			c.Next()
			return
		}
		reject := func(status int, code string) {
			c.AbortWithStatusJSON(status, gin.H{"success": false, "message": "CAPTCHA verification failed. Refresh it and try again.", "code": code})
		}
		if common.TurnstileSiteKey == "" || common.TurnstileSecretKey == "" {
			reject(http.StatusServiceUnavailable, "LOGIN_CAPTCHA_UNAVAILABLE")
			return
		}
		token := c.GetHeader("X-Turnstile-Token")
		if token == "" {
			token = c.Query("turnstile")
		}
		if token == "" {
			reject(http.StatusBadRequest, "LOGIN_CAPTCHA_REQUIRED")
			return
		}
		// Cloudflare documents a maximum token size of 2048 characters.
		if len(token) > 2048 {
			reject(http.StatusBadRequest, "LOGIN_CAPTCHA_INVALID")
			return
		}
		fields := url.Values{"secret": {common.TurnstileSecretKey}, "response": {token}, "remoteip": {c.ClientIP()}}
		request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(fields.Encode()))
		if err != nil {
			reject(http.StatusServiceUnavailable, "LOGIN_CAPTCHA_UNAVAILABLE")
			return
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		result, err := turnstileClient.Do(request)
		if err != nil {
			reject(http.StatusServiceUnavailable, "LOGIN_CAPTCHA_UNAVAILABLE")
			return
		}
		defer result.Body.Close()
		var verified turnstileCheckResponse
		if result.StatusCode != http.StatusOK || common.DecodeJson(io.LimitReader(result.Body, 64*1024), &verified) != nil {
			reject(http.StatusServiceUnavailable, "LOGIN_CAPTCHA_UNAVAILABLE")
			return
		}
		if !verified.Success {
			reject(http.StatusBadRequest, "LOGIN_CAPTCHA_INVALID")
			return
		}
		c.Next()
	}
}
