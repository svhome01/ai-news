package thumbnail

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
)

const (
	thumbWidth  = 320
	thumbHeight = 180
	thumbDir    = "/data/thumbnails"
)

// Store manages article thumbnail images stored in thumbDir.
type Store struct {
	baseDir string
}

// New creates a Store and ensures the thumbnail directory exists.
func New() (*Store, error) {
	s := &Store{baseDir: thumbDir}
	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return nil, fmt.Errorf("thumbnail mkdir: %w", err)
	}
	return s, nil
}

// DownloadAndSave downloads the image at remoteURL, resizes it to 320×180 (16:9 centre-crop),
// saves it as JPEG at baseDir/{articleID}.jpg, and returns the HTTP serving path
// "/thumbnails/{articleID}.jpg".
// Returns ("", nil) if remoteURL is empty.
func (s *Store) DownloadAndSave(ctx context.Context, articleID int64, remoteURL string) (string, error) {
	if remoteURL == "" {
		return "", nil
	}

	img, err := s.fetchImage(ctx, remoteURL)
	if err != nil {
		// Non-fatal: thumbnail failure must not block article saving.
		return "", fmt.Errorf("fetch thumbnail %q: %w", remoteURL, err)
	}

	resized := imaging.Fill(img, thumbWidth, thumbHeight, imaging.Center, imaging.Lanczos)

	localPath := filepath.Join(s.baseDir, fmt.Sprintf("%d.jpg", articleID))
	if err := imaging.Save(resized, localPath); err != nil {
		return "", fmt.Errorf("save thumbnail: %w", err)
	}

	return fmt.Sprintf("/thumbnails/%d.jpg", articleID), nil
}

// Delete removes the thumbnail file for the given article ID.
// Missing files are silently ignored.
func (s *Store) Delete(articleID int64) error {
	path := filepath.Join(s.baseDir, fmt.Sprintf("%d.jpg", articleID))
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Store) fetchImage(ctx context.Context, url string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ai-news/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}
