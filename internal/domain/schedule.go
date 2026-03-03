package domain

// Schedule represents one cron schedule entry for a scrape or generate job.
// Each enabled entry registers an independent cron job: "0 <Minute> <Hour> * * *".
type Schedule struct {
	ID      int64
	Type    string // "scrape" or "generate"
	Hour    int    // 0–23
	Minute  int    // 0–59
	Enabled bool
}
