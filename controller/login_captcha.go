package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func GetLoginCaptcha(c *gin.Context) {
	if common.LoginCaptchaMode != common.LoginCaptchaImage {
		c.JSON(http.StatusConflict, gin.H{"success": false, "code": "LOGIN_CAPTCHA_MODE_CHANGED"})
		return
	}
	id, image, err := common.NewLoginCaptcha(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "code": "LOGIN_CAPTCHA_UNAVAILABLE"})
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "image": image})
}
