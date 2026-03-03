package domain

// AppSettings holds global application settings.
// The table enforces a single row (id = 1).
type AppSettings struct {
	ID                 int
	VoicevoxSpeedScale float64
	GeminiModel        string
	RetentionDays      int
	UpdatedAt          string
}
