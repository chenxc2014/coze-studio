package entity

// APISource 保存在已有 parse_rule JSON 内，无需修改知识库表结构。
type APISource struct {
	URL           string   `json:"url"`
	IntervalDays  int      `json:"interval_days"`
	Columns       []string `json:"columns"`
	Hash          string   `json:"hash"`
	PendingHash   string   `json:"pending_hash,omitempty"`
	PreviousURI   string   `json:"previous_uri,omitempty"`
	LastFetchedAt int64    `json:"last_fetched_at"`
	LastSuccessAt int64    `json:"last_success_at"`
	NextRunAt     int64    `json:"next_run_at"`
	LastError     string   `json:"last_error,omitempty"`
}
