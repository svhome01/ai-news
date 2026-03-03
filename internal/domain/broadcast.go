package domain

// BroadcastType distinguishes single-category episodes from cross-category digests.
type BroadcastType string

const (
	BroadcastTypeEpisode BroadcastType = "episode"
	BroadcastTypeDigest  BroadcastType = "digest"
)

// Broadcast is a generated audio episode stored as an MP3 on the Samba share.
type Broadcast struct {
	ID            int64
	PipelineRunID int64
	Category      string        // e.g. "tech", "business", "digest"
	BroadcastType BroadcastType
	Title         string // e.g. "Tech News - 2026-02-26 朝"
	Script        *string       // full DJ-style script (episode only)
	FilePath      string        // SMB-relative path used as the go-smb2 key
	FileURL       *string       // HTTP URL returned in API responses (media_url)
	DurationSec   *int          // populated via ffprobe after encoding
	CreatedAt     string
}
