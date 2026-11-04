package common

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"github.com/dchest/captcha"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const LoginCaptchaImage = "image"
const LoginCaptchaTurnstile = "turnstile"
const loginCaptchaPurpose = "login-captcha:"

var LoginCaptchaMode = LoginCaptchaImage

func loginCaptchaHash(id, answer string) string {
	mac := hmac.New(sha256.New, []byte(SessionSecret))
	mac.Write([]byte(loginCaptchaPurpose + id + ":" + answer))
	return string(mac.Sum(nil))
}

// NewLoginCaptcha uses the existing verification lifetime and shares challenges
// through Redis when enabled. Neither the answer nor its digest is returned.
func NewLoginCaptcha(ctx context.Context) (id, image string, err error) {
	id = uuid.NewString()
	digits := captcha.RandomDigits(captcha.DefaultLen)
	var png bytes.Buffer
	if _, err = captcha.NewImage(id, digits, captcha.StdWidth, captcha.StdHeight).WriteTo(&png); err != nil {
		return "", "", err
	}
	answer := make([]byte, len(digits))
	for i, digit := range digits {
		answer[i] = '0' + digit
	}
	hash := loginCaptchaHash(id, string(answer))
	if RedisEnabled {
		if RDB == nil {
			return "", "", errors.New("captcha Redis client is unavailable")
		}
		err = RDB.Set(ctx, loginCaptchaPurpose+id, hash, time.Duration(VerificationValidMinutes)*time.Minute).Err()
		if err != nil {
			return "", "", err
		}
	} else {
		RegisterVerificationCodeWithKey(id, hash, loginCaptchaPurpose)
	}
	return id, "data:image/png;base64," + base64.StdEncoding.EncodeToString(png.Bytes()), nil
}

// Every submitted challenge is consumed atomically, including wrong answers.
// Lua supports older Redis versions that do not implement GETDEL.
func VerifyLoginCaptcha(ctx context.Context, id, answer string) (bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return false, nil
	}
	var stored string
	if RedisEnabled {
		if RDB == nil {
			return false, errors.New("captcha Redis client is unavailable")
		}
		value, err := RDB.Eval(ctx, `local v = redis.call('GET', KEYS[1]); redis.call('DEL', KEYS[1]); return v`, []string{loginCaptchaPurpose + id}).Text()
		if err != nil && !errors.Is(err, redis.Nil) {
			return false, err
		}
		stored = value
	} else {
		verificationMutex.Lock()
		value, exists := verificationMap[loginCaptchaPurpose+id]
		delete(verificationMap, loginCaptchaPurpose+id)
		verificationMutex.Unlock()
		if exists && time.Since(value.time) < time.Duration(VerificationValidMinutes)*time.Minute {
			stored = value.code
		}
	}
	if len(answer) != captcha.DefaultLen || stored == "" {
		return false, nil
	}
	for _, digit := range answer {
		if digit < '0' || digit > '9' {
			return false, nil
		}
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(loginCaptchaHash(id, answer))) == 1, nil
}
