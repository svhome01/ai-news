package storage

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"

	"github.com/hirochachacha/go-smb2"
)

// MusicStore handles MP3 read/write/delete operations over SMB.
type MusicStore struct {
	host      string // e.g. "192.168.0.22"
	user      string
	pass      string
	share     string // e.g. "Music"
	musicPath string // e.g. "ai-news"
}

// New creates a MusicStore from the given environment values.
func New(host, user, pass, share, musicPath string) *MusicStore {
	return &MusicStore{
		host:      host,
		user:      user,
		pass:      pass,
		share:     share,
		musicPath: musicPath,
	}
}

// dial opens an SMB session, mounts the configured share, and returns a cleanup function.
// The caller must call the cleanup func after use.
func (s *MusicStore) dial() (*smb2.Share, func(), error) {
	conn, err := net.Dial("tcp", s.host+":445")
	if err != nil {
		return nil, nil, fmt.Errorf("smb dial: %w", err)
	}

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     s.user,
			Password: s.pass,
		},
	}
	session, err := d.Dial(conn)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("smb session: %w", err)
	}

	share, err := session.Mount(s.share)
	if err != nil {
		session.Logoff()
		conn.Close()
		return nil, nil, fmt.Errorf("smb mount %s: %w", s.share, err)
	}

	cleanup := func() {
		share.Umount()
		session.Logoff()
		conn.Close()
	}
	return share, cleanup, nil
}

// SMBPath returns the SMB-relative path for the given category and filename.
// e.g. "ai-news/tech/tech-news-20260226.mp3"
func (s *MusicStore) SMBPath(category, filename string) string {
	return path.Join(s.musicPath, category, filename)
}

// SaveMP3 writes mp3Data to the SMB share at filePath.
// filePath is relative to the share root (e.g. "ai-news/tech/foo.mp3").
func (s *MusicStore) SaveMP3(filePath string, mp3Data []byte) error {
	share, cleanup, err := s.dial()
	if err != nil {
		return err
	}
	defer cleanup()

	// Ensure parent directories exist.
	dir := path.Dir(filePath)
	if err := share.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("smb mkdir %s: %w", dir, err)
	}

	f, err := share.Create(filePath)
	if err != nil {
		return fmt.Errorf("smb create %s: %w", filePath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, bytes.NewReader(mp3Data)); err != nil {
		return fmt.Errorf("smb write %s: %w", filePath, err)
	}
	return nil
}

// StreamMP3 reads the MP3 at filePath from SMB and streams it to w.
func (s *MusicStore) StreamMP3(w http.ResponseWriter, filePath string) error {
	share, cleanup, err := s.dial()
	if err != nil {
		return err
	}
	defer cleanup()

	f, err := share.Open(filePath)
	if err != nil {
		return fmt.Errorf("smb open %s: %w", filePath, err)
	}
	defer f.Close()

	w.Header().Set("Content-Type", "audio/mpeg")
	_, err = io.Copy(w, f)
	return err
}

// ReadMP3 downloads the MP3 at filePath from SMB and returns its bytes.
func (s *MusicStore) ReadMP3(filePath string) ([]byte, error) {
	share, cleanup, err := s.dial()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	f, err := share.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("smb open %s: %w", filePath, err)
	}
	defer f.Close()

	return io.ReadAll(f)
}

// DeleteMP3 removes the MP3 at filePath from SMB.
// Returns nil if the file does not exist.
func (s *MusicStore) DeleteMP3(filePath string) error {
	share, cleanup, err := s.dial()
	if err != nil {
		return err
	}
	defer cleanup()

	if err := share.Remove(filePath); err != nil {
		// Ignore "file not found" errors.
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("smb remove %s: %w", filePath, err)
	}
	return nil
}

// isNotFound returns true if the SMB error indicates a missing file/path.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "STATUS_OBJECT_NAME_NOT_FOUND") ||
		contains(msg, "STATUS_NO_SUCH_FILE") ||
		contains(msg, "The system cannot find")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
