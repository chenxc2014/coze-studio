package taskcenter

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/coze-dev/coze-studio/backend/pkg/lang/signal"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

func (s *Service) Start(ctx context.Context) {
	if !s.Ready {
		logs.Warnf("task center schema is not installed; task center is unavailable")
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	go func() { signal.WaitExit(); cancel() }()
	go func() {
		for workerCtx.Err() == nil {
			if err := s.lead(workerCtx); err != nil && workerCtx.Err() == nil {
				logs.Errorf("task center worker lost leadership or database access")
			}
			select {
			case <-workerCtx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}

func (s *Service) lead(ctx context.Context) error {
	database, e := s.db.DB()
	if e != nil {
		return e
	}
	conn, e := database.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	var acquired int
	if e = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", s.lockName).Scan(&acquired); e != nil {
		return e
	}
	if acquired != 1 {
		return nil
	}
	defer func() {
		releaseCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_, _ = conn.ExecContext(releaseCtx, "SELECT RELEASE_LOCK(?)", s.lockName)
	}()
	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	bytes := make([]byte, 16)
	if _, e = rand.Read(bytes); e != nil {
		return e
	}
	key := hex.EncodeToString(bytes)
	if e = s.recoverRuns(leaderCtx, "", 0); e != nil {
		return e
	}
	type completed struct {
		id  int64
		err error
	}
	done := make(chan completed, 2)
	active := map[int64]bool{}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer func() {
		cancel()
		for len(active) > 0 {
			event := <-done
			delete(active, event.id)
		}
	}()
	for {
		select {
		case <-leaderCtx.Done():
			return leaderCtx.Err()
		case event := <-done:
			delete(active, event.id)
			s.mu.Lock()
			delete(s.cancels, event.id)
			s.mu.Unlock()
			if event.err != nil {
				if e = s.recoverRuns(leaderCtx, key, event.id); e != nil {
					return e
				}
			}
		case <-ticker.C:
			if e = checkLeader(leaderCtx, conn, s.lockName); e != nil {
				return e
			}
			for id := range active {
				var status string
				if e = s.db.WithContext(leaderCtx).Model(&Run{}).Select("status").Where("id = ?", id).Scan(&status).Error; e != nil {
					return e
				}
				if status == Cancelling {
					s.mu.Lock()
					stop := s.cancels[id]
					s.mu.Unlock()
					if stop != nil {
						stop()
					}
				}
			}
			for len(active) < 2 {
				var run Run
				e = s.db.WithContext(leaderCtx).Where("status = ?", Queued).Order("created_at,id").First(&run).Error
				if errors.Is(e, gorm.ErrRecordNotFound) {
					break
				}
				if e != nil {
					return e
				}
				changed := s.db.WithContext(leaderCtx).Model(&Run{}).Where("id = ? AND status = ?", run.ID, Queued).Updates(map[string]any{"status": Running, "started_at": time.Now().UnixMilli(), "worker_key": key})
				if changed.Error != nil {
					return changed.Error
				}
				if changed.RowsAffected == 0 {
					continue
				}
				run.WorkerKey = key
				runCtx, stop := context.WithCancel(leaderCtx)
				active[run.ID] = true
				s.mu.Lock()
				s.cancels[run.ID] = stop
				s.mu.Unlock()
				go func(run Run) { err := s.processRun(runCtx, &run, key); done <- completed{run.ID, err} }(run)
			}
		}
	}
}

func checkLeader(ctx context.Context, conn *sql.Conn, name string) error {
	check, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var owns int
	if err := conn.QueryRowContext(check, "SELECT COALESCE(IS_USED_LOCK(?) = CONNECTION_ID(),0)", name).Scan(&owns); err != nil {
		return err
	}
	if owns != 1 {
		return fmt.Errorf("task center leadership lost")
	}
	return nil
}

func (s *Service) recoverRuns(ctx context.Context, key string, onlyID int64) error {
	runs := []Run{}
	q := s.db.WithContext(ctx).Where("status IN ?", []string{Running, Cancelling})
	if key != "" {
		q = q.Where("worker_key = ?", key)
	}
	if onlyID > 0 {
		q = q.Where("id = ?", onlyID)
	}
	if err := q.Find(&runs).Error; err != nil {
		return err
	}
	for _, run := range runs {
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var current Run
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", run.ID).Error; err != nil {
				return err
			}
			if key != "" && current.WorkerKey != key {
				return nil
			}
			if current.Status != Running && current.Status != Cancelling {
				return nil
			}
			status, message := Interrupted, "执行过程被中断，结果未确认；请核对工作流副作用后手动重试"
			if current.Status == Cancelling {
				status = Cancelled
				message = "任务已取消"
				if err := tx.Model(&Item{}).Where("run_id = ? AND status = ?", run.ID, Queued).Updates(map[string]any{"status": Cancelled, "finished_at": time.Now().UnixMilli()}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&Item{}).Where("run_id = ? AND status = ?", run.ID, Running).Updates(map[string]any{"status": status, "error": message, "finished_at": time.Now().UnixMilli()}).Error; err != nil {
				return err
			}
			if err := tx.Model(&Run{}).Where("id = ?", run.ID).Update("worker_key", "").Error; err != nil {
				return err
			}
			if err := s.refreshRun(ctx, tx, run.ID); err != nil {
				return err
			}
			return tx.Model(&Run{}).Where("id = ? AND status = ?", run.ID, Running).Update("status", Queued).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) claim(ctx context.Context, runID int64, key string) (*Item, error) {
	var result *Item
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run Run
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; e != nil {
			return e
		}
		if run.Status != Running || run.WorkerKey != key {
			return nil
		}
		var item Item
		e := tx.Where("run_id = ? AND status = ?", runID, Queued).Order("position").First(&item).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		}
		if e != nil {
			return e
		}
		item.Status = Running
		item.WorkerKey = key
		item.StartedAt = time.Now().UnixMilli()
		if e = tx.Model(&Item{}).Where("id = ? AND status = ?", item.ID, Queued).Updates(map[string]any{"status": Running, "worker_key": key, "started_at": item.StartedAt}).Error; e != nil {
			return e
		}
		result = &item
		return nil
	})
	return result, err
}

func (s *Service) processRun(ctx context.Context, run *Run, key string) error {
	var task Task
	if err := s.db.WithContext(ctx).First(&task, "id = ?", run.TaskID).Error; err != nil {
		return err
	}
	s.inflate(ctx, &task)
	group, workCtx := errgroup.WithContext(ctx)
	for i := 0; i < task.Concurrency; i++ {
		group.Go(func() error {
			for workCtx.Err() == nil {
				item, err := s.claim(workCtx, run.ID, key)
				if err != nil {
					return err
				}
				if item == nil {
					return nil
				}
				input, err := InputFromJSON(task.Fields, item.InputJSON)
				var result Result
				if err == nil {
					itemCtx, stop := context.WithTimeout(workCtx, time.Duration(task.TimeoutSeconds)*time.Second)
					result, err = s.runner.Execute(itemCtx, &task, input)
					stop()
				}
				status, message := Succeeded, ""
				if err != nil {
					status = Failed
					message = err.Error()
				}
				if len(result.Output) > MaxOutputBytes {
					status = Failed
					message = "结果超过 32 KiB，请在工作流中保存文件并返回下载链接"
					result.Output = ""
				}
				if workCtx.Err() != nil {
					status = Interrupted
					message = "执行中断，结果未确认"
				}
				if len(message) > 2048 {
					message = string([]rune(message)[:min(len([]rune(message)), 512)])
				}
				commitCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				err = s.db.WithContext(commitCtx).Transaction(func(tx *gorm.DB) error {
					var current Run
					if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", run.ID).Error; e != nil {
						return e
					}
					if current.WorkerKey != key {
						return nil
					}
					if current.Status == Cancelling {
						status = Cancelled
						message = "任务已取消"
					}
					changed := tx.Model(&Item{}).Where("id = ? AND status = ? AND worker_key = ?", item.ID, Running, key).Updates(map[string]any{"status": status, "output_json": result.Output, "error": message, "execution_id": result.ExecutionID, "finished_at": time.Now().UnixMilli()})
					if changed.Error != nil {
						return changed.Error
					}
					if changed.RowsAffected > 0 {
						if e := tx.Model(&Run{}).Where("id = ?", run.ID).Update("stored_bytes", gorm.Expr("stored_bytes + ?", len(result.Output)+len(message))).Error; e != nil {
							return e
						}
					}
					return s.refreshRun(commitCtx, tx, run.ID)
				})
				stop()
				if err != nil {
					return err
				}
			}
			return workCtx.Err()
		})
	}
	err := group.Wait()
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.refreshRun(ctx, tx, run.ID) })
}

func (s *Service) refreshRun(ctx context.Context, tx *gorm.DB, id int64) error {
	var run Run
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", id).Error; err != nil {
		return err
	}
	var counts []struct {
		Status string
		N      int
	}
	if err := tx.Model(&Item{}).Select("status,COUNT(*) AS n").Where("run_id = ?", id).Group("status").Scan(&counts).Error; err != nil {
		return err
	}
	values := map[string]int{}
	for _, count := range counts {
		values[count.Status] = count.N
	}
	status := summarize(values, run.Status)
	updates := map[string]any{"status": status, "succeeded": values[Succeeded], "failed": values[Failed], "cancelled": values[Cancelled], "interrupted": values[Interrupted]}
	if Terminal(status) {
		updates["finished_at"] = time.Now().UnixMilli()
	}
	return tx.Model(&Run{}).Where("id = ?", id).Updates(updates).Error
}

func summarize(counts map[string]int, current string) string {
	if counts[Running] > 0 || counts[Queued] > 0 {
		if current == Cancelling {
			return Cancelling
		}
		return Running
	}
	if current == Cancelling || counts[Cancelled] > 0 {
		return Cancelled
	}
	if counts[Interrupted] > 0 {
		return Interrupted
	}
	if counts[Failed] > 0 {
		if counts[Succeeded] > 0 {
			return "partial"
		}
		return Failed
	}
	return "completed"
}
