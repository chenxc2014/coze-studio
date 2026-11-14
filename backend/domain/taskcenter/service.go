package taskcenter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type Service struct {
	db       *gorm.DB
	ids      idgen.IDGenerator
	runner   Runner
	Ready    bool
	mu       sync.Mutex
	cancels  map[int64]context.CancelFunc
	exports  chan struct{}
	lockName string
}

func New(db *gorm.DB, ids idgen.IDGenerator, runner Runner) *Service {
	s := &Service{db: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}), ids: ids, runner: runner, cancels: map[int64]context.CancelFunc{}, exports: make(chan struct{}, 1)}
	s.lockName = "taskcenter_" + fingerprint(db.Migrator().CurrentDatabase())[:24]
	s.Ready = db.Migrator().HasTable(&Task{}) && db.Migrator().HasTable(&Run{}) && db.Migrator().HasTable(&Item{})
	return s
}

func ID(text string) (int64, error) {
	n, e := strconv.ParseInt(text, 10, 64)
	if e != nil || n <= 0 {
		return 0, invalid("资源 ID 不正确")
	}
	return n, nil
}
func fingerprint(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

var requestKey = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

func (s *Service) ownerLock(tx *gorm.DB, owner int64) error {
	// 同一创建者的配额检查与提交串行化，不修改账号数据。
	var id int64
	if err := tx.Raw("SELECT id FROM user WHERE id = ? FOR UPDATE", owner).Scan(&id).Error; err != nil {
		return err
	}
	if id != owner {
		return &Error{403, "账号不可用"}
	}
	return nil
}

func (s *Service) nextIDs(ctx context.Context, n int) ([]int64, error) {
	result := make([]int64, 0, n)
	for n > 0 {
		size := min(n, 200)
		ids, e := s.ids.GenMultiIDs(ctx, size)
		if e != nil {
			return nil, e
		}
		if len(ids) != size {
			return nil, fmt.Errorf("task center ID allocation failed")
		}
		result = append(result, ids...)
		n -= size
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, owner, space int64, req CreateRequest) (*Task, error) {
	if !requestKey.MatchString(req.RequestKey) {
		return nil, invalid("请求标识不正确，请重新打开创建窗口")
	}
	hash := fingerprint(req)
	var existing Task
	e := s.db.WithContext(ctx).Where("owner_id = ? AND client_key = ?", owner, req.RequestKey).First(&existing).Error
	if e == nil {
		if existing.SpaceID != space || existing.RequestHash != hash {
			return nil, conflict("同一请求标识不能用于不同任务")
		}
		return s.inflate(ctx, &existing), nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	name := strings.TrimSpace(req.Name)
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 {
		return nil, invalid("任务名称应为 1～100 个字符")
	}
	if req.Kind != "batch" && req.Kind != "async" {
		return nil, invalid("任务类型不正确")
	}
	if req.Concurrency == 0 {
		req.Concurrency = 2
	}
	if req.Concurrency < 1 || req.Concurrency > 4 {
		return nil, invalid("并发数应为 1～4")
	}
	if req.Kind == "async" {
		req.Concurrency = 1
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 3600
	}
	if req.TimeoutSeconds < 10 || req.TimeoutSeconds > 86400 {
		return nil, invalid("单条超时应为 10～86400 秒")
	}
	wfID, e := ID(req.WorkflowID)
	if e != nil {
		return nil, e
	}
	wf, e := s.runner.Resolve(ctx, owner, space, wfID, req.Version)
	if e != nil {
		return nil, e
	}
	var rows []map[string]any
	if req.Kind == "batch" {
		rows, e = ParseCSV(wf.Fields, req.CSVBase64)
		if e != nil {
			return nil, e
		}
	}
	fields, e := json.Marshal(wf.Fields)
	if e != nil {
		return nil, e
	}
	ids, e := s.nextIDs(ctx, 1)
	if e != nil {
		return nil, e
	}
	task := &Task{ID: ids[0], SpaceID: space, OwnerID: owner, Name: name, Kind: req.Kind, WorkflowID: wf.ID, WorkflowName: wf.Name, Version: wf.Version, FieldsJSON: string(fields), Concurrency: req.Concurrency, TimeoutSeconds: req.TimeoutSeconds, ClientKey: req.RequestKey, RequestHash: hash}
	e = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.ownerLock(tx, owner); err != nil {
			return err
		}
		var again Task
		err := tx.Where("owner_id = ? AND client_key = ?", owner, req.RequestKey).First(&again).Error
		if err == nil {
			if again.RequestHash != hash || again.SpaceID != space {
				return conflict("请求标识已使用")
			}
			*task = again
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err = tx.Model(&Task{}).Where("owner_id = ?", owner).Count(&count).Error; err != nil {
			return err
		}
		if count >= 200 {
			return conflict("最多保留 200 个任务，请先删除不再需要的任务")
		}
		if err = tx.Create(task).Error; err != nil {
			return err
		}
		if req.Kind == "batch" {
			_, err = s.createRunTx(ctx, tx, task, rows, nil, 0, req.RequestKey, hash)
		}
		return err
	})
	if e != nil {
		return nil, e
	}
	return s.inflate(ctx, task), nil
}

func (s *Service) inflate(ctx context.Context, task *Task) *Task {
	_ = json.Unmarshal([]byte(task.FieldsJSON), &task.Fields)
	if task.Fields == nil {
		task.Fields = []Field{}
	}
	if task.LatestRunID > 0 {
		var run Run
		if s.db.WithContext(ctx).First(&run, "id = ?", task.LatestRunID).Error == nil {
			task.LatestRun = &run
		}
	}
	return task
}

func (s *Service) Get(ctx context.Context, space, id int64) (*Task, error) {
	var task Task
	e := s.db.WithContext(ctx).Where("id = ? AND space_id = ?", id, space).First(&task).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, &Error{404, "任务不存在或无权访问"}
	}
	if e != nil {
		return nil, e
	}
	return s.inflate(ctx, &task), nil
}

func (s *Service) List(ctx context.Context, space, actor int64, kind, status, query string, mine bool, page, size int) ([]*Task, int64, error) {
	if kind != "batch" && kind != "async" {
		return nil, 0, invalid("任务类型不正确")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}
	q := s.db.WithContext(ctx).Model(&Task{}).Where("space_id = ? AND kind = ?", space, kind)
	if mine {
		q = q.Where("owner_id = ?", actor)
	}
	if query != "" {
		q = q.Where("name LIKE ?", "%"+query+"%")
	}
	if status == "not_started" {
		q = q.Where("latest_run_id = 0")
	} else if status != "" {
		q = q.Where("latest_run_id IN (SELECT id FROM taskcenter_runs WHERE status = ?)", status)
	}
	var total int64
	if e := q.Count(&total).Error; e != nil {
		return nil, 0, e
	}
	tasks := []*Task{}
	if e := q.Order("updated_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&tasks).Error; e != nil {
		return nil, 0, e
	}
	for _, task := range tasks {
		s.inflate(ctx, task)
		task.Fields = nil
	}
	return tasks, total, nil
}

func (s *Service) Detail(ctx context.Context, space, taskID, runID int64, page, size int, status string, historyPage ...int) (*Detail, error) {
	task, e := s.Get(ctx, space, taskID)
	if e != nil {
		return nil, e
	}
	runs := []*Run{}
	history := 1
	if len(historyPage) > 0 && historyPage[0] > 0 {
		history = historyPage[0]
	}
	var runsTotal int64
	if e = s.db.WithContext(ctx).Model(&Run{}).Where("task_id = ?", task.ID).Count(&runsTotal).Error; e != nil {
		return nil, e
	}
	if e = s.db.WithContext(ctx).Where("task_id = ?", task.ID).Order("id DESC").Offset((history - 1) * 20).Limit(20).Find(&runs).Error; e != nil {
		return nil, e
	}
	result := &Detail{Task: task, Runs: runs, RunsTotal: runsTotal, Items: []*Item{}}
	if runID == 0 {
		runID = task.LatestRunID
	}
	if runID == 0 {
		return result, nil
	}
	var run Run
	if e = s.db.WithContext(ctx).Where("id = ? AND task_id = ?", runID, task.ID).First(&run).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, &Error{404, "执行记录不存在"}
		}
		return nil, e
	}
	result.Run = &run
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := s.db.WithContext(ctx).Model(&Item{}).Where("run_id = ?", runID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if e = q.Count(&result.ItemsTotal).Error; e != nil {
		return nil, e
	}
	if e = q.Order("position ASC").Offset((page - 1) * size).Limit(size).Find(&result.Items).Error; e != nil {
		return nil, e
	}
	return result, nil
}

func (s *Service) createRunTx(ctx context.Context, tx *gorm.DB, task *Task, rows []map[string]any, positions []int, retryOf int64, key, hash string) (*Run, error) {
	var exists int64
	if err := tx.Model(&Task{}).Where("id = ? AND owner_id = ? AND space_id = ?", task.ID, task.OwnerID, task.SpaceID).Count(&exists).Error; err != nil {
		return nil, err
	}
	if exists != 1 {
		return nil, &Error{404, "任务已删除，请刷新列表"}
	}
	if !requestKey.MatchString(key) {
		return nil, invalid("请求标识不正确")
	}
	var existing Run
	err := tx.Where("owner_id = ? AND client_key = ?", task.OwnerID, key).First(&existing).Error
	if err == nil {
		if existing.TaskID != task.ID || existing.RequestHash != hash {
			return nil, conflict("请求标识已用于其他执行")
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if len(rows) < 1 || len(rows) > MaxRows {
		return nil, invalid("执行行数应为 1～1000")
	}
	var active int64
	if err = tx.Model(&Run{}).Where("owner_id = ? AND status IN ?", task.OwnerID, []string{Queued, Running, Cancelling}).Count(&active).Error; err != nil {
		return nil, err
	}
	if active >= 10 {
		return nil, conflict("最多同时保留 10 个排队或执行中的任务")
	}
	var used int64
	if err = tx.Model(&Run{}).Select("COALESCE(SUM(stored_bytes + GREATEST(total - succeeded - failed - cancelled - interrupted, 0) * ?), 0)", MaxOutputBytes).Where("owner_id = ?", task.OwnerID).Scan(&used).Error; err != nil {
		return nil, err
	}
	inputs := make([]string, len(rows))
	var inputBytes int64
	for i, row := range rows {
		data, e := json.Marshal(row)
		if e != nil {
			return nil, invalid("参数无法序列化")
		}
		if len(data) > MaxInputBytes {
			return nil, invalid("单条输入超过 16 KiB")
		}
		inputs[i] = string(data)
		inputBytes += int64(len(data))
	}
	if used+inputBytes+int64(len(rows))*MaxOutputBytes > MaxUserBytes {
		return nil, conflict("任务数据配额为 256 MiB，请先删除不再需要的任务或执行记录")
	}
	ids, err := s.nextIDs(ctx, len(rows)+1)
	if err != nil {
		return nil, err
	}
	run := &Run{ID: ids[0], TaskID: task.ID, OwnerID: task.OwnerID, Status: Queued, Total: len(rows), StoredBytes: inputBytes, RetryOf: retryOf, ClientKey: key, RequestHash: hash}
	if err = tx.Create(run).Error; err != nil {
		return nil, err
	}
	items := make([]*Item, len(rows))
	for i := range rows {
		position := i + 1
		if len(positions) > 0 {
			position = positions[i]
		}
		items[i] = &Item{ID: ids[i+1], RunID: run.ID, Position: position, Status: Queued, InputJSON: inputs[i]}
	}
	if err = tx.CreateInBatches(items, 100).Error; err != nil {
		return nil, err
	}
	if err = tx.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{"latest_run_id": run.ID, "updated_at": time.Now().UnixMilli()}).Error; err != nil {
		return nil, err
	}
	task.LatestRunID = run.ID
	return run, nil
}

func (s *Service) Trigger(ctx context.Context, actor, space, taskID int64, inputJSON, key string) (*Run, error) {
	task, e := s.Get(ctx, space, taskID)
	if e != nil {
		return nil, e
	}
	if task.OwnerID != actor {
		return nil, &Error{403, "只有创建者可以执行此任务"}
	}
	if task.Kind != "async" {
		return nil, invalid("批量任务请通过 CSV 创建或重试未成功项")
	}
	if inputJSON == "" {
		inputJSON = "{}"
	}
	input, e := InputFromJSON(task.Fields, inputJSON)
	if e != nil {
		return nil, e
	}
	var result *Run
	e = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.ownerLock(tx, actor); err != nil {
			return err
		}
		var err error
		result, err = s.createRunTx(ctx, tx, task, []map[string]any{input}, nil, 0, key, fingerprint(input))
		return err
	})
	return result, e
}

func (s *Service) Retry(ctx context.Context, actor, space, taskID, runID int64, key string) (*Run, error) {
	detail, e := s.Detail(ctx, space, taskID, runID, 1, 1, "")
	if e != nil {
		return nil, e
	}
	if detail.Task.OwnerID != actor {
		return nil, &Error{403, "只有创建者可以重试"}
	}
	if detail.Run == nil || !Terminal(detail.Run.Status) {
		return nil, conflict("执行结束后才能重试")
	}
	items := []*Item{}
	if e = s.db.WithContext(ctx).Where("run_id = ? AND status IN ?", detail.Run.ID, []string{Failed, Interrupted, Cancelled}).Order("position").Find(&items).Error; e != nil {
		return nil, e
	}
	if len(items) == 0 {
		return nil, invalid("没有需要重试的行")
	}
	inputs := make([]map[string]any, len(items))
	positions := make([]int, len(items))
	for i, item := range items {
		inputs[i], e = InputFromJSON(detail.Task.Fields, item.InputJSON)
		if e != nil {
			return nil, e
		}
		positions[i] = item.Position
	}
	var result *Run
	e = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.ownerLock(tx, actor); err != nil {
			return err
		}
		var err error
		result, err = s.createRunTx(ctx, tx, detail.Task, inputs, positions, detail.Run.ID, key, fingerprint(detail.Run.ID))
		return err
	})
	return result, e
}

func (s *Service) Cancel(ctx context.Context, actor, space, taskID, runID int64) error {
	detail, e := s.Detail(ctx, space, taskID, runID, 1, 1, "")
	if e != nil {
		return e
	}
	if detail.Task.OwnerID != actor {
		return &Error{403, "只有创建者可以取消"}
	}
	if detail.Run == nil || Terminal(detail.Run.Status) {
		return conflict("该执行已经结束")
	}
	e = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Run{}).Where("id = ? AND status IN ?", detail.Run.ID, []string{Queued, Running}).Update("status", Cancelling).Error; err != nil {
			return err
		}
		if err := tx.Model(&Item{}).Where("run_id = ? AND status = ?", detail.Run.ID, Queued).Updates(map[string]any{"status": Cancelled, "finished_at": time.Now().UnixMilli()}).Error; err != nil {
			return err
		}
		return s.refreshRun(ctx, tx, detail.Run.ID)
	})
	if e == nil {
		s.mu.Lock()
		cancel := s.cancels[detail.Run.ID]
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	return e
}

func (s *Service) Delete(ctx context.Context, actor, space, taskID int64) error {
	task, e := s.Get(ctx, space, taskID)
	if e != nil {
		return e
	}
	if task.OwnerID != actor {
		return &Error{403, "只有创建者可以删除任务"}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.ownerLock(tx, actor); err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&Run{}).Where("task_id = ? AND status IN ?", taskID, []string{Queued, Running, Cancelling}).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return conflict("请先取消正在执行或排队的任务")
		}
		if err := tx.Where("run_id IN (SELECT id FROM taskcenter_runs WHERE task_id = ?)", taskID).Delete(&Item{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", taskID).Delete(&Run{}).Error; err != nil {
			return err
		}
		return tx.Delete(&Task{}, "id = ?", taskID).Error
	})
}

func (s *Service) Export(ctx context.Context, space, taskID, runID int64) ([]byte, error) {
	select {
	case s.exports <- struct{}{}:
		defer func() { <-s.exports }()
	default:
		return nil, &Error{429, "已有导出正在生成，请稍后重试"}
	}
	detail, e := s.Detail(ctx, space, taskID, runID, 1, 1, "")
	if e != nil {
		return nil, e
	}
	if detail.Run == nil {
		return nil, invalid("尚无执行记录")
	}
	if !Terminal(detail.Run.Status) {
		return nil, conflict("执行结束后可以导出")
	}
	items := []*Item{}
	if e = s.db.WithContext(ctx).Where("run_id = ?", detail.Run.ID).Order("position ASC").Limit(MaxRows).Find(&items).Error; e != nil {
		return nil, e
	}
	return ResultsCSV(detail.Task.Fields, items)
}
