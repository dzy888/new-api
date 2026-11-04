package common

import (
	"context"
	"encoding/base64"
	"image/png"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginCaptchaSingleUseExpiryAndSharedRedis(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			oldEnabled, oldClient := RedisEnabled, RDB
			RedisEnabled = backend == "redis"
			t.Cleanup(func() { RedisEnabled, RDB = oldEnabled, oldClient })
			var server *miniredis.Miniredis
			if RedisEnabled {
				server = miniredis.RunT(t)
				RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
				t.Cleanup(func() { _ = RDB.Close() })
			}
			seed := func(age time.Duration) string {
				id := uuid.NewString()
				hash := loginCaptchaHash(id, "123456")
				if RedisEnabled {
					require.NoError(t, RDB.Set(context.Background(), loginCaptchaPurpose+id, hash, time.Duration(VerificationValidMinutes)*time.Minute).Err())
					if age > 0 {
						server.FastForward(age)
					}
				} else {
					verificationMutex.Lock()
					verificationMap[loginCaptchaPurpose+id] = verificationValue{code: hash, time: time.Now().Add(-age)}
					verificationMutex.Unlock()
				}
				return id
			}
			t.Run("correct answer only succeeds once", func(t *testing.T) {
				id := seed(0)
				ok, err := VerifyLoginCaptcha(context.Background(), id, "123456")
				require.NoError(t, err)
				assert.True(t, ok)
				ok, err = VerifyLoginCaptcha(context.Background(), id, "123456")
				require.NoError(t, err)
				assert.False(t, ok)
			})
			t.Run("wrong and malformed answers consume the challenge", func(t *testing.T) {
				for _, answer := range []string{"654321", "", "12a456"} {
					id := seed(0)
					ok, err := VerifyLoginCaptcha(context.Background(), id, answer)
					require.NoError(t, err)
					assert.False(t, ok)
					ok, err = VerifyLoginCaptcha(context.Background(), id, "123456")
					require.NoError(t, err)
					assert.False(t, ok)
				}
			})
			t.Run("expired answer is rejected", func(t *testing.T) {
				id := seed(time.Duration(VerificationValidMinutes)*time.Minute + time.Second)
				ok, err := VerifyLoginCaptcha(context.Background(), id, "123456")
				require.NoError(t, err)
				assert.False(t, ok)
			})
			t.Run("concurrent replay has exactly one winner", func(t *testing.T) {
				id := seed(0)
				var winners atomic.Int32
				var failures atomic.Int32
				var wg sync.WaitGroup
				for range 8 {
					wg.Go(func() {
						ok, err := VerifyLoginCaptcha(context.Background(), id, "123456")
						if err != nil {
							failures.Add(1)
						}
						if ok {
							winners.Add(1)
						}
					})
				}
				wg.Wait()
				assert.Equal(t, int32(0), failures.Load())
				assert.Equal(t, int32(1), winners.Load())
			})
			t.Run("generation returns PNG and an opaque ID", func(t *testing.T) {
				id, image, err := NewLoginCaptcha(context.Background())
				require.NoError(t, err)
				_, err = uuid.Parse(id)
				require.NoError(t, err)
				bytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(image, "data:image/png;base64,"))
				require.NoError(t, err)
				_, err = png.Decode(strings.NewReader(string(bytes)))
				require.NoError(t, err)
			})
		})
	}
}

func TestLoginCaptchaRedisOutageFailsClosed(t *testing.T) {
	oldEnabled, oldClient := RedisEnabled, RDB
	RedisEnabled = true
	RDB = nil
	defer func() { RedisEnabled, RDB = oldEnabled, oldClient }()
	_, _, err := NewLoginCaptcha(context.Background())
	require.Error(t, err)
	ok, err := VerifyLoginCaptcha(context.Background(), uuid.NewString(), "123456")
	require.Error(t, err)
	assert.False(t, ok)
}
