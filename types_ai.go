package serpkite

import (
	"encoding/json"
	"time"
)

// Types of the AI endpoints: answer (+ streaming) and research. Like types.go
// they mirror backend/api/serp-api.yaml and are checked by contract_test.go.

// ── Answer ───────────────────────────────────────────────────────────────

// Answer depths ([AnswerParams].Depth).
const (
	// DepthFast answers from the result page (snippets, answer box, knowledge panel).
	DepthFast = "fast"
	// DepthStandard (default) also reads the top NumSources pages.
	DepthStandard = "standard"
)

// AnswerParams are the parameters of [Client.Answer] and [Client.AnswerStream].
type AnswerParams struct {
	// Q is the question (required).
	Q string `json:"q"`
	// Depth is DepthFast or DepthStandard (default).
	Depth string `json:"depth,omitempty"`
	// NumSources is how many pages DepthStandard reads (1-5, default 3).
	NumSources int `json:"num_sources,omitempty"`
	// Stream is set by [Client.AnswerStream]; [Client.Answer] clears it.
	Stream bool `json:"stream,omitempty"`
	// OutputSchema is a JSON Schema (at most 16 KB, e.g. a map or a
	// json.RawMessage); the answer then comes back as a JSON value in
	// [AnswerResponse].Output. Not with streaming.
	OutputSchema any    `json:"output_schema,omitempty"`
	Country      string `json:"country,omitempty"`
	Language     string `json:"language,omitempty"`
	Location     string `json:"location,omitempty"`
	UULE         string `json:"uule,omitempty"`
	Time         string `json:"time,omitempty"`
	TBS          string `json:"tbs,omitempty"`
	Device       string `json:"device,omitempty"`
	Safe         string `json:"safe,omitempty"`
	Autocorrect  *bool  `json:"autocorrect,omitempty"`
	// MaxAge accepts a cached search up to this many seconds old.
	MaxAge int    `json:"max_age,omitempty"`
	Engine Engine `json:"engine,omitempty"`
	// IncludeDomains, ExcludeDomains, BoostDomains, StartDate and EndDate work
	// as in [SearchParams].
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	BoostDomains   []string `json:"boost_domains,omitempty"`
	StartDate      string   `json:"start_date,omitempty"`
	EndDate        string   `json:"end_date,omitempty"`
}

// AnswerSource is a result an answer may cite, numbered as in the prompt.
type AnswerSource struct {
	Index  int    `json:"index"`
	Title  string `json:"title"`
	Link   string `json:"link"`
	Domain string `json:"domain,omitempty"`
	// Date is the publish date (ISO 8601 when known, else as shown).
	Date string `json:"date,omitempty"`
}

// AnswerCredits splits [Meta].CreditsUsed of an answer into the search and the model part.
type AnswerCredits struct {
	Search float64 `json:"search,omitempty"`
	Answer float64 `json:"answer,omitempty"`
}

// AnswerMeta is the meta of an answer: [Meta] plus the credit split.
type AnswerMeta struct {
	Meta
	Credits *AnswerCredits `json:"credits,omitempty"`
}

// AnswerResponse is returned by [Client.Answer].
type AnswerResponse struct {
	Request RequestEcho `json:"request"`
	// Answer is Markdown with "[n]" citation markers. Nil with OutputSchema, or
	// when there was nothing to answer from (Meta.Warnings says so).
	Answer *string `json:"answer"`
	// Output is the JSON value matching OutputSchema; decode it with DecodeOutput.
	Output json.RawMessage `json:"output,omitempty"`
	// Citations are the sources the answer cites, in first-cited order.
	Citations []Citation     `json:"citations"`
	Results   []AnswerSource `json:"results"`
	Meta      AnswerMeta     `json:"meta"`
}

// Text returns the answer text, or "" when there is none.
func (r *AnswerResponse) Text() string {
	if r.Answer == nil {
		return ""
	}
	return *r.Answer
}

// DecodeOutput unmarshals Output into v. It returns false when there is no output.
func (r *AnswerResponse) DecodeOutput(v any) (bool, error) { return decodeRaw(r.Output, v) }

// AnswerStreamResults is the first event of a stream ("results").
type AnswerStreamResults struct {
	Request RequestEcho    `json:"request"`
	Results []AnswerSource `json:"results"`
}

// AnswerStreamDelta is the next piece of answer text ("delta").
type AnswerStreamDelta struct {
	Text string `json:"text"`
}

// AnswerStreamDone is the last event of a stream ("done"): the final answer
// (citation markers checked) and meta with the total credits.
type AnswerStreamDone struct {
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
	Meta      AnswerMeta `json:"meta"`
}

// Answer stream event types ([AnswerEvent].Type).
const (
	AnswerEventResults = "results"
	AnswerEventDelta   = "delta"
	AnswerEventDone    = "done"
)

// AnswerEvent is one event of an [AnswerStream]. Exactly one of Results,
// Delta and Done is set, matching Type.
type AnswerEvent struct {
	Type    string
	Results *AnswerStreamResults
	Delta   *AnswerStreamDelta
	Done    *AnswerStreamDone
}

// ── Research ─────────────────────────────────────────────────────────────

// Research efforts ([ResearchParams].Effort): more rounds, queries and pages
// read per step, at a higher flat price (https://serpkite.com/pricing).
const (
	EffortLite     = "lite"
	EffortStandard = "standard"
	EffortDeep     = "deep"
)

// ResearchParams are the parameters of [Client.Research].
type ResearchParams struct {
	// Input is the research task in plain words (required).
	Input string `json:"input"`
	// Effort is EffortLite, EffortStandard (default) or EffortDeep.
	Effort string `json:"effort,omitempty"`
	// OutputSchema is a JSON Schema (at most 16 KB); the report is also
	// returned as JSON in [ResearchResult].Output.
	OutputSchema   any      `json:"output_schema,omitempty"`
	Country        string   `json:"country,omitempty"`
	Language       string   `json:"language,omitempty"`
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	// WebhookURL receives the signed research.completed event (default: the account webhook).
	WebhookURL string `json:"webhook_url,omitempty"`
}

// ResearchProgress is the progress of a running research task.
type ResearchProgress struct {
	Round   int `json:"round,omitempty"`
	Rounds  int `json:"rounds,omitempty"`
	Queries int `json:"queries,omitempty"`
	Sources int `json:"sources,omitempty"`
	// Step is searching, reviewing or writing.
	Step string `json:"step,omitempty"`
}

// ResearchSource is one numbered source of a research report.
type ResearchSource struct {
	Index       int    `json:"index"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	Domain      string `json:"domain,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	// Query is the search that found it.
	Query string `json:"query,omitempty"`
}

// ResearchStats summarises a research task.
type ResearchStats struct {
	Searches int `json:"searches,omitempty"`
	Sources  int `json:"sources,omitempty"`
	Rounds   int `json:"rounds,omitempty"`
}

// ResearchResult is the result of a research task. A failed or canceled task
// may carry only Queries.
type ResearchResult struct {
	Input  string `json:"input"`
	Effort string `json:"effort"`
	// Report is Markdown with "[n]" citation markers.
	Report    string           `json:"report"`
	Citations []Citation       `json:"citations"`
	Sources   []ResearchSource `json:"sources"`
	// Queries are every search the research ran.
	Queries []string       `json:"queries"`
	Stats   *ResearchStats `json:"stats,omitempty"`
	LLM     *LLMUsage      `json:"llm,omitempty"`
	// Output is the JSON value of OutputSchema (null when it could not be
	// produced: OutputError); decode it with DecodeOutput.
	Output      json.RawMessage `json:"output,omitempty"`
	OutputError string          `json:"output_error,omitempty"`
}

// DecodeOutput unmarshals Output into v. It returns false when there is no output.
func (r *ResearchResult) DecodeOutput(v any) (bool, error) { return decodeRaw(r.Output, v) }

// ResearchTask is returned by [Client.GetResearch].
type ResearchTask struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CreditsReserved float64    `json:"credits_reserved"`
	CreditsUsed     float64    `json:"credits_used"`
	// WebhookStatus is pending, delivered, failed or nil.
	WebhookStatus *string           `json:"webhook_status,omitempty"`
	Progress      *ResearchProgress `json:"progress,omitempty"`
	// Result is set once the report is written.
	Result *ResearchResult `json:"result,omitempty"`
	Error  *TaskError      `json:"error,omitempty"`
}

// Finished reports whether the research completed, failed or was canceled.
func (t *ResearchTask) Finished() bool { return taskFinished(t.Status) }
