package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

const recordVersion = 1

var validID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type DisplayMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type Record struct {
	Version     int              `json:"version"`
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	Provider    string           `json:"provider"`
	Model       string           `json:"model"`
	History     []types.Message  `json:"history,omitempty"`
	Messages    []DisplayMessage `json:"messages,omitempty"`
	TokenCount  int              `json:"token_count,omitempty"`
	TokenBytes  int              `json:"token_bytes,omitempty"`
	SessionCost float64          `json:"session_cost,omitempty"`
}

type Summary struct {
	ID        string
	Title     string
	UpdatedAt time.Time
	Provider  string
	Model     string
}

type Store struct {
	dir string
	now func() time.Time
}

func NewStore(dir string, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{dir: dir, now: now}
}

func (s *Store) Create(title, provider, model string) (*Record, error) {
	id, err := newID()
	if err != nil {
		return nil, fmt.Errorf("session: generate id: %w", err)
	}
	now := s.now().UTC()
	record := &Record{
		Version:   recordVersion,
		ID:        id,
		Title:     normalizeTitle(title),
		CreatedAt: now,
		UpdatedAt: now,
		Provider:  provider,
		Model:     model,
	}
	if err := s.Save(record); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Store) Save(record *Record) error {
	if record == nil {
		return errors.New("session: save nil record")
	}
	path, err := s.path(record.ID)
	if err != nil {
		return err
	}
	if err := s.ensureSafeDir(); err != nil {
		return err
	}
	record.Version = recordVersion
	if record.CreatedAt.IsZero() {
		record.CreatedAt = s.now().UTC()
	}
	record.UpdatedAt = s.now().UTC()
	record.Title = normalizeTitle(record.Title)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("session: encode %q: %w", record.ID, err)
	}
	tmp, err := os.CreateTemp(s.dir, ".session-*.tmp")
	if err != nil {
		return fmt.Errorf("session: create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("session: secure temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("session: write temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("session: sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("session: close temporary file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("session: replace %q: %w", record.ID, err)
	}
	return nil
}

func (s *Store) Load(id string) (*Record, error) {
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSafeDir(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session: load %q: %w", id, err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("session: decode %q: %w", id, err)
	}
	if record.Version != recordVersion || record.ID != id {
		return nil, fmt.Errorf("session: invalid record %q", id)
	}
	return &record, nil
}

func (s *Store) List() ([]Summary, error) {
	if _, err := os.Stat(s.dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err := s.ensureSafeDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session: list store: %w", err)
	}
	var summaries []Summary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !validID.MatchString(id) {
			continue
		}
		record, err := s.Load(id)
		if err != nil {
			continue
		}
		summaries = append(summaries, Summary{
			ID: record.ID, Title: record.Title, UpdatedAt: record.UpdatedAt,
			Provider: record.Provider, Model: record.Model,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	return summaries, nil
}

func (s *Store) ensureSafeDir() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("session: create store: %w", err)
	}
	abs, err := filepath.Abs(s.dir)
	if err != nil {
		return fmt.Errorf("session: resolve store path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("session: resolve store symlinks: %w", err)
	}
	same := filepath.Clean(abs) == filepath.Clean(resolved)
	if runtime.GOOS == "windows" {
		same = strings.EqualFold(filepath.Clean(abs), filepath.Clean(resolved))
	}
	if !same {
		return fmt.Errorf("session: store directory must not be a symlink")
	}
	return nil
}

func (s *Store) Rename(id, title string) error {
	record, err := s.Load(id)
	if err != nil {
		return err
	}
	record.Title = normalizeTitle(title)
	return s.Save(record)
}

func (s *Store) Delete(id string) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := s.ensureSafeDir(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("session: delete %q: %w", id, err)
	}
	return nil
}

func (s *Store) path(id string) (string, error) {
	if !validID.MatchString(id) {
		return "", fmt.Errorf("session: invalid id %q", id)
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func newID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

func normalizeTitle(title string) string {
	title = strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if title == "" {
		return "New session"
	}
	runes := []rune(title)
	if len(runes) > 80 {
		return string(runes[:77]) + "..."
	}
	return title
}
