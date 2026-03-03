package domain

// RunStatus is the lifecycle state of a pipeline execution.
type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

// JobType distinguishes a scrape-only run from a generate (TTS + MP3) run.
type JobType string

const (
	JobTypeScrape   JobType = "scrape"
	JobTypeGenerate JobType = "generate"
)

// TriggeredBy records what initiated the pipeline run.
type TriggeredBy string

const (
	TriggeredByCron TriggeredBy = "cron"
	TriggeredByAPI  TriggeredBy = "api"
	TriggeredByUI   TriggeredBy = "ui"
)

// PipelineRun records one execution of the scrape or generate pipeline.
type PipelineRun struct {
	ID                int64
	JobType           JobType
	Status            RunStatus
	TriggeredBy       TriggeredBy
	CurrentStep       *string // progress label for HTMX polling UI
	ArticlesCollected *int    // set by ScrapeJob
	ArticlesSelected  *int    // set by GenerateJob (0 = no eligible articles)
	ErrorMessage      *string
	StartedAt         string
	FinishedAt        *string
}
