package taskcenter

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	MaxRows        = 1000
	MaxCSVBytes    = 2 << 20
	MaxInputBytes  = 16 << 10
	MaxOutputBytes = 32 << 10
	MaxUserBytes   = 256 << 20
	Queued         = "queued"
	Running        = "running"
	Succeeded      = "succeeded"
	Failed         = "failed"
	Cancelled      = "cancelled"
	Interrupted    = "interrupted"
	Cancelling     = "cancelling"
)

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string                 { return e.Message }
func invalid(format string, args ...any) error { return &Error{400, fmt.Sprintf(format, args...)} }
func conflict(message string) error            { return &Error{409, message} }

type Field struct {
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	Required     bool            `json:"required,omitempty"`
	Description  string          `json:"description,omitempty"`
	Schema       json.RawMessage `json:"schema,omitempty"`
	DefaultValue json.RawMessage `json:"defaultValue,omitempty"`
}

type Workflow struct {
	ID       int64    `json:"id,string"`
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	AppID    *int64   `json:"app_id,string,omitempty"`
	Fields   []Field  `json:"fields"`
	Versions []string `json:"versions,omitempty"`
}

type Task struct {
	ID             int64   `gorm:"primaryKey;autoIncrement:false" json:"id,string"`
	SpaceID        int64   `json:"space_id,string"`
	OwnerID        int64   `json:"owner_id,string"`
	Name           string  `json:"name"`
	Kind           string  `json:"kind"`
	WorkflowID     int64   `json:"workflow_id,string"`
	WorkflowName   string  `json:"workflow_name"`
	Version        string  `json:"version"`
	FieldsJSON     string  `json:"-"`
	ClientKey      string  `json:"-"`
	RequestHash    string  `json:"-"`
	Concurrency    int     `json:"concurrency"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	LatestRunID    int64   `json:"latest_run_id,string"`
	CreatedAt      int64   `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt      int64   `gorm:"autoUpdateTime:milli" json:"updated_at"`
	LatestRun      *Run    `gorm:"-" json:"latest_run,omitempty"`
	Fields         []Field `gorm:"-" json:"fields,omitempty"`
}

func (Task) TableName() string { return "taskcenter_tasks" }

type Run struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id,string"`
	TaskID      int64  `json:"task_id,string"`
	OwnerID     int64  `json:"owner_id,string"`
	Status      string `json:"status"`
	Total       int    `json:"total"`
	Succeeded   int    `json:"succeeded"`
	Failed      int    `json:"failed"`
	Cancelled   int    `json:"cancelled"`
	Interrupted int    `json:"interrupted"`
	StoredBytes int64  `json:"stored_bytes"`
	RetryOf     int64  `json:"retry_of,string"`
	ClientKey   string `json:"-"`
	RequestHash string `json:"-"`
	CreatedAt   int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	WorkerKey   string `json:"-"`
	StartedAt   int64  `json:"started_at"`
	FinishedAt  int64  `json:"finished_at"`
}

func (Run) TableName() string { return "taskcenter_runs" }

type Item struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id,string"`
	RunID       int64  `json:"run_id,string"`
	Position    int    `json:"position"`
	Status      string `json:"status"`
	InputJSON   string `json:"input_json"`
	OutputJSON  string `json:"output_json"`
	Error       string `json:"error"`
	ExecutionID int64  `json:"execution_id,string"`
	WorkerKey   string `json:"-"`
	StartedAt   int64  `json:"started_at"`
	FinishedAt  int64  `json:"finished_at"`
}

func (Item) TableName() string { return "taskcenter_items" }

type Result struct {
	ExecutionID int64
	Output      string
}
type Runner interface {
	Resolve(context.Context, int64, int64, int64, string) (*Workflow, error)
	Execute(context.Context, *Task, map[string]any) (Result, error)
}

type CreateRequest struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	WorkflowID     string `json:"workflow_id"`
	Version        string `json:"version"`
	CSVBase64      string `json:"csv_base64"`
	Concurrency    int    `json:"concurrency"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	RequestKey     string `json:"request_key"`
}

type Detail struct {
	Task       *Task   `json:"task"`
	Runs       []*Run  `json:"runs"`
	RunsTotal  int64   `json:"runs_total"`
	Run        *Run    `json:"run,omitempty"`
	Items      []*Item `json:"items"`
	ItemsTotal int64   `json:"items_total"`
}

func Terminal(status string) bool {
	return status != Queued && status != Running && status != Cancelling
}
