package nsq

import "time"

func startMessageTouch(touch func(), interval time.Duration) func() {
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				touch()
			}
		}
	}()
	return func() { close(stop); <-stopped }
}
