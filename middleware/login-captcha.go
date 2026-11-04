package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func LoginCaptchaRateLimit() gin.HandlerFunc {
	return rateLimitFactory(common.CriticalRateLimitNum, common.CriticalRateLimitDuration, "LOGIN_CAPTCHA")
}

func LoginCaptchaCheck() gin.HandlerFunc {
	turnstile := TurnstileCheck(true)
	return func(c *gin.Context) {
		switch common.LoginCaptchaMode {
		case common.LoginCaptchaTurnstile:
			turnstile(c)
		case common.LoginCaptchaImage:
			id, answer := c.GetHeader("X-Login-Captcha-Id"), c.GetHeader("X-Login-Captcha-Code")
			if id == "" {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "CAPTCHA verification failed. Refresh it and try again.", "code": "LOGIN_CAPTCHA_REQUIRED"})
				return
			}
			valid, err := common.VerifyLoginCaptcha(c.Request.Context(), id, answer)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "CAPTCHA verification failed. Refresh it and try again.", "code": "LOGIN_CAPTCHA_UNAVAILABLE"})
				return
			}
			if !valid {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "CAPTCHA verification failed. Refresh it and try again.", "code": "LOGIN_CAPTCHA_INVALID"})
				return
			}
			c.Next()
		default:
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "CAPTCHA verification failed. Refresh it and try again.", "code": "LOGIN_CAPTCHA_UNAVAILABLE"})
		}
	}
}
