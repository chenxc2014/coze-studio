package openauth

import (
	"errors"
	"strconv"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/openauth/openapiauth/entity"
)

func resolveTokenExpiration(duration string, custom int64, now time.Time) (int64, error) {
	switch duration {
	case "permanent":
		return entity.PermanentExpiration, nil
	case "customize":
		if custom <= now.Unix() {
			return 0, errors.New("invalid expiration time")
		}
		return custom, nil
	default:
		days, err := strconv.ParseInt(duration, 10, 64)
		// 防止负数有效期和 time.Duration 溢出绕过到期检查。
		if err != nil || days <= 0 || days > int64((1<<63-1)/int64(24*time.Hour)) {
			return 0, errors.New("invalid expireDay")
		}
		return now.Add(time.Duration(days) * 24 * time.Hour).Unix(), nil
	}
}
