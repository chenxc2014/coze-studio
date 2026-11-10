package openauth

import (
	"testing"
	"time"
)

func TestResolveTokenExpiration(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, tc := range []struct {
		name, duration string
		custom, want   int64
		invalid        bool
	}{
		{"永久", "permanent", 0, -1, false},
		{"永久忽略自定义日期", "permanent", now.Unix() - 1, -1, false},
		{"一天", "1", 0, now.Add(24 * time.Hour).Unix(), false},
		{"三十天", "30", 0, now.Add(30 * 24 * time.Hour).Unix(), false},
		{"自定义", "customize", now.Unix() + 60, now.Unix() + 60, false},
		{"自定义不能伪造永久", "customize", -1, 0, true},
		{"已过期", "customize", now.Unix() - 1, 0, true},
		{"到期边界", "customize", now.Unix(), 0, true},
		{"零天", "0", 0, 0, true},
		{"负天数", "-1", 0, 0, true},
		{"溢出", "9223372036854775807", 0, 0, true},
		{"未知选项", "unknown", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveTokenExpiration(tc.duration, tc.custom, now)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("有效期结果=%d，错误=%v", got, err)
			}
		})
	}
}
