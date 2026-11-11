package nsq

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestMessageTouchContinuesUntilStopped(t *testing.T) {
	var count atomic.Int64
	touched := make(chan struct{}, 8)
	stop := startMessageTouch(func() {
		count.Add(1)
		select {
		case touched <- struct{}{}:
		default:
		}
	}, 5*time.Millisecond)
	for i := 0; i < 2; i++ {
		select {
		case <-touched:
		case <-time.After(time.Second):
			stop()
			t.Fatal("长任务期间未续期消息")
		}
	}
	stop()
	before := count.Load()
	time.Sleep(20 * time.Millisecond)
	if count.Load() != before {
		t.Fatal("任务结束后仍在续期消息")
	}
}
