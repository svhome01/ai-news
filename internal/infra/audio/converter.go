package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	id3 "github.com/bogem/id3v2/v2"
)

// EncodedMP3 holds the result of WAV→MP3 conversion.
type EncodedMP3 struct {
	Data        []byte
	DurationSec int
}

// WAVToMP3 converts WAV bytes to MP3 bytes, attaches ID3 tags, and measures duration.
// title is used as the ID3 title tag. Returns EncodedMP3 or error.
func WAVToMP3(ctx context.Context, wavData []byte, title string) (*EncodedMP3, error) {
	tmpDir, err := os.MkdirTemp("", "ai-news-wav2mp3-*")
	if err != nil {
		return nil, fmt.Errorf("mktempdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	wavPath := filepath.Join(tmpDir, "input.wav")
	mp3Path := filepath.Join(tmpDir, "output.mp3")

	if err := os.WriteFile(wavPath, wavData, 0600); err != nil {
		return nil, fmt.Errorf("write wav: %w", err)
	}

	// Convert WAV → MP3 using ffmpeg (192kbps, mono).
	cmd := exec.CommandContext(ctx,
		"ffmpeg", "-y",
		"-i", wavPath,
		"-codec:a", "libmp3lame",
		"-b:a", "192k",
		"-ac", "1",
		mp3Path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg encode: %w\noutput: %s", err, out)
	}

	// Attach ID3v2 tags.
	if err := addID3Tags(mp3Path, title); err != nil {
		// Non-fatal: log and proceed without tags.
		_ = err
	}

	// Measure duration.
	durationSec, err := ffprobeDuration(ctx, mp3Path)
	if err != nil {
		durationSec = 0
	}

	data, err := os.ReadFile(mp3Path)
	if err != nil {
		return nil, fmt.Errorf("read mp3: %w", err)
	}

	return &EncodedMP3{Data: data, DurationSec: durationSec}, nil
}

// addID3Tags writes a basic ID3 title tag to the MP3 file at path.
func addID3Tags(path, title string) error {
	tag, err := id3.Open(path, id3.Options{Parse: true})
	if err != nil {
		return err
	}
	defer tag.Close()
	tag.SetTitle(title)
	return tag.Save()
}

// ProbeDuration returns the duration in seconds of the media file at path.
func ProbeDuration(ctx context.Context, path string) (int, error) {
	return ffprobeDuration(ctx, path)
}

// ffprobeDuration returns the duration in seconds of the given media file.
func ffprobeDuration(ctx context.Context, path string) (int, error) {
	out, err := exec.CommandContext(ctx,
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int(f), nil
}

// WAVsToMP3 synthesizes multiple WAV byte slices into a single MP3 file.
// wavSlices are concatenated via ffmpeg in order before encoding to MP3.
// Returns the encoded MP3 data and its duration in seconds.
func WAVsToMP3(ctx context.Context, wavSlices [][]byte, title string) (*EncodedMP3, error) {
	if len(wavSlices) == 0 {
		return nil, fmt.Errorf("no wav slices provided")
	}
	tmpDir, err := os.MkdirTemp("", "ai-news-wavs2mp3-*")
	if err != nil {
		return nil, fmt.Errorf("mktempdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write each WAV slice.
	var wavPaths []string
	for i, data := range wavSlices {
		p := filepath.Join(tmpDir, fmt.Sprintf("part%03d.wav", i))
		if err := os.WriteFile(p, data, 0600); err != nil {
			return nil, fmt.Errorf("write wav part %d: %w", i, err)
		}
		wavPaths = append(wavPaths, p)
	}

	// Build ffmpeg concat list.
	listPath := filepath.Join(tmpDir, "concat.txt")
	var sb strings.Builder
	for _, p := range wavPaths {
		sb.WriteString("file '")
		sb.WriteString(p)
		sb.WriteString("'\n")
	}
	if err := os.WriteFile(listPath, []byte(sb.String()), 0600); err != nil {
		return nil, fmt.Errorf("write concat list: %w", err)
	}

	mp3Path := filepath.Join(tmpDir, "output.mp3")
	cmd := exec.CommandContext(ctx,
		"ffmpeg", "-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-codec:a", "libmp3lame",
		"-b:a", "192k",
		"-ac", "1",
		mp3Path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg encode: %w\noutput: %s", err, out)
	}

	if err := addID3Tags(mp3Path, title); err != nil {
		_ = err // non-fatal
	}

	durationSec, err := ffprobeDuration(ctx, mp3Path)
	if err != nil {
		durationSec = 0
	}

	data, err := os.ReadFile(mp3Path)
	if err != nil {
		return nil, fmt.Errorf("read mp3: %w", err)
	}
	return &EncodedMP3{Data: data, DurationSec: durationSec}, nil
}

// ConcatMP3 concatenates multiple MP3 files (given as byte slices, in order) into one.
// The caller is responsible for creating and removing tmpDir.
func ConcatMP3(ctx context.Context, mp3Slices [][]byte, tmpDir string) ([]byte, error) {
	if len(mp3Slices) == 0 {
		return nil, fmt.Errorf("no mp3 slices to concat")
	}

	// Write each slice to a temp file.
	var inputPaths []string
	for i, data := range mp3Slices {
		p := filepath.Join(tmpDir, fmt.Sprintf("part%03d.mp3", i))
		if err := os.WriteFile(p, data, 0600); err != nil {
			return nil, fmt.Errorf("write part %d: %w", i, err)
		}
		inputPaths = append(inputPaths, p)
	}

	// Build concat list file for ffmpeg.
	listPath := filepath.Join(tmpDir, "concat.txt")
	var sb strings.Builder
	for _, p := range inputPaths {
		sb.WriteString("file '")
		sb.WriteString(p)
		sb.WriteString("'\n")
	}
	if err := os.WriteFile(listPath, []byte(sb.String()), 0600); err != nil {
		return nil, fmt.Errorf("write concat list: %w", err)
	}

	outPath := filepath.Join(tmpDir, "digest.mp3")
	cmd := exec.CommandContext(ctx,
		"ffmpeg", "-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c", "copy",
		outPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg concat: %w\noutput: %s", err, out)
	}

	return os.ReadFile(outPath)
}
