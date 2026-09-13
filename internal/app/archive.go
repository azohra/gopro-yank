package app

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	schemaVersion = 1
	controlName   = ".gopro-yank"
)

type FileRecord struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	ZIPCRC32  string `json:"zip_crc32,omitempty"`
	ZIPMember string `json:"zip_member,omitempty"`
	Adopted   bool   `json:"adopted,omitempty"`
}

type IntegrityIssue struct {
	MediaID string `json:"media_id"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type IntegrityState struct {
	Status     string           `json:"status"`
	VerifiedAt string           `json:"verified_at,omitempty"`
	CheckedAt  string           `json:"checked_at,omitempty"`
	Issues     []IntegrityIssue `json:"issues,omitempty"`
}

type ReplacedArtifact struct {
	Path       string `json:"path"`
	RetainedAt string `json:"retained_at"`
	Reason     string `json:"reason"`
}

type ItemRecord struct {
	MediaID           string             `json:"media_id"`
	Status            string             `json:"status"`
	SourcePresent     *bool              `json:"source_present"`
	SourceSnapshot    string             `json:"source_snapshot,omitempty"`
	Source            map[string]any     `json:"source,omitempty"`
	Filename          string             `json:"filename,omitempty"`
	FileSize          int64              `json:"file_size,omitempty"`
	CapturedAt        string             `json:"captured_at,omitempty"`
	CreatedAt         string             `json:"created_at,omitempty"`
	MediaType         string             `json:"media_type,omitempty"`
	Reason            string             `json:"reason,omitempty"`
	Error             string             `json:"error,omitempty"`
	Files             []FileRecord       `json:"files"`
	ArchivedAt        string             `json:"archived_at,omitempty"`
	VerifiedAt        string             `json:"verified_at,omitempty"`
	AdoptedAt         string             `json:"adopted_at,omitempty"`
	Integrity         *IntegrityState    `json:"integrity,omitempty"`
	ReplacedArtifacts []ReplacedArtifact `json:"replaced_artifacts,omitempty"`
}

type SnapshotRecord struct {
	ID         string `json:"id"`
	CapturedAt string `json:"captured_at"`
	ItemCount  int    `json:"item_count"`
	Path       string `json:"path"`
}

type Manifest struct {
	SchemaVersion  int                    `json:"schema_version"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
	Source         map[string]any         `json:"source"`
	LatestSnapshot string                 `json:"latest_snapshot,omitempty"`
	Snapshots      []SnapshotRecord       `json:"snapshots"`
	Items          map[string]*ItemRecord `json:"items"`
	LegacyAdoption map[string]any         `json:"legacy_adoption,omitempty"`
}

type Archive struct {
	Root          string
	ControlDir    string
	ManifestPath  string
	SnapshotsDir  string
	StagingDir    string
	ReportPath    string
	ChecksumsPath string
	Exists        bool
	Data          Manifest
	mu            sync.RWMutex
}

type VerificationResult struct {
	CheckedItems int
	CheckedFiles int
	CheckedBytes int64
	Issues       []IntegrityIssue
}

func (v VerificationResult) OK() bool { return len(v.Issues) == 0 }

type Summary struct {
	Archived          int
	Manual            int
	ReplacedArtifacts int
	Files             int
	Blockers          int
	Bytes             int64
}

type AdoptionResult struct {
	AdoptedItems   int
	AdoptedFiles   int
	ManualItems    int
	AttentionItems int
	BytesHashed    int64
}

func nowUTC() string { return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339) }

func NewArchive(root string) (*Archive, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	a := &Archive{Root: abs}
	a.ControlDir = filepath.Join(abs, controlName)
	a.ManifestPath = filepath.Join(a.ControlDir, "manifest.json")
	a.SnapshotsDir = filepath.Join(a.ControlDir, "snapshots")
	a.StagingDir = filepath.Join(a.ControlDir, "staging")
	a.ReportPath = filepath.Join(a.ControlDir, "report.html")
	a.ChecksumsPath = filepath.Join(a.ControlDir, "checksums.sha256")
	if _, err := os.Stat(a.ManifestPath); err == nil {
		a.Exists = true
		payload, err := os.ReadFile(a.ManifestPath)
		if err != nil {
			return nil, fmt.Errorf("read archive manifest: %w", err)
		}
		if err := json.Unmarshal(payload, &a.Data); err != nil {
			return nil, fmt.Errorf("parse archive manifest: %w", err)
		}
		if a.Data.SchemaVersion != schemaVersion {
			return nil, fmt.Errorf("unsupported archive schema %d; expected %d", a.Data.SchemaVersion, schemaVersion)
		}
		if a.Data.Items == nil {
			return nil, errors.New("archive manifest has no items collection")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else {
		now := nowUTC()
		a.Data = Manifest{
			SchemaVersion: schemaVersion,
			CreatedAt:     now,
			UpdatedAt:     now,
			Source:        map[string]any{"provider": "gopro-cloud"},
			Snapshots:     []SnapshotRecord{},
			Items:         map[string]*ItemRecord{},
		}
	}
	return a, nil
}

func atomicWrite(path string, payload []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	if err := file.Chmod(mode); err != nil {
		file.Close()
		_ = os.Remove(temporary)
		return err
	}
	if _, err = file.Write(payload); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err = replaceFile(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func (a *Archive) saveLocked() error {
	if err := os.MkdirAll(a.ControlDir, 0o755); err != nil {
		return err
	}
	a.Data.UpdatedAt = nowUTC()
	payload, err := json.MarshalIndent(a.Data, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := atomicWrite(a.ManifestPath, payload, 0o644); err != nil {
		return err
	}
	a.Exists = true
	return a.writeChecksumsLocked()
}

func (a *Archive) Save() error { a.mu.Lock(); defer a.mu.Unlock(); return a.saveLocked() }

func secureJoin(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("archive path escapes its root: %s", relative)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive path escapes its root: %s", relative)
	}
	resolvedRoot := root
	if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil {
		resolvedRoot = resolved
	}
	probe := candidate
	for {
		if _, statErr := os.Lstat(probe); statErr == nil {
			resolvedProbe, resolveErr := filepath.EvalSymlinks(probe)
			if resolveErr != nil {
				return "", resolveErr
			}
			resolvedRel, relErr := filepath.Rel(resolvedRoot, resolvedProbe)
			if relErr != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("archive path escapes its root through a symlink: %s", relative)
			}
			break
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(probe)
		if probe == root || parent == probe || len(parent) < len(root) {
			break
		}
		probe = parent
	}
	return candidate, nil
}

func sha256File(path string) (string, int64, error) {
	return sha256FileContext(context.Background(), path)
}

func sha256FileContext(ctx context.Context, path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hash := sha256.New()
	buffer := make([]byte, 4*1024*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return "", size, err
		}
		read, readErr := file.Read(buffer)
		if read > 0 {
			if _, err := hash.Write(buffer[:read]); err != nil {
				return "", size, err
			}
			size += int64(read)
		}
		if errors.Is(readErr, io.EOF) {
			return hex.EncodeToString(hash.Sum(nil)), size, nil
		}
		if readErr != nil {
			return "", size, readErr
		}
	}
}

func boolPointer(value bool) *bool { return &value }

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "_", "-"))
	for _, fragment := range []string{"authorization", "cookie", "credential", "password", "secret", "signature", "token"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func sanitizeSource(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clean := make(map[string]any, len(typed))
		for key, child := range typed {
			if sensitiveKey(key) {
				clean[key] = "[redacted]"
			} else {
				clean[key] = sanitizeSource(child)
			}
		}
		return clean
	case []any:
		clean := make([]any, len(typed))
		for i, child := range typed {
			clean[i] = sanitizeSource(child)
		}
		return clean
	case string:
		parsed, err := url.Parse(typed)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return typed
		}
		query := parsed.Query()
		for key := range query {
			if sensitiveKey(key) {
				query.Set(key, "[redacted]")
			}
		}
		parsed.RawQuery = query.Encode()
		return parsed.String()
	default:
		return value
	}
}

func (a *Archive) RecordSnapshot(items []MediaItem, userID string, full map[string]map[string]any) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now().UTC()
	id := now.Format("20060102T150405.000000000Z")
	for _, record := range a.Data.Items {
		record.SourcePresent = boolPointer(false)
	}
	snapshotItems := make([]map[string]any, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		raw := item.Raw
		if candidate, ok := full[item.ID]; ok {
			raw = candidate
		}
		clean, _ := sanitizeSource(raw).(map[string]any)
		record := a.Data.Items[item.ID]
		if record == nil {
			record = &ItemRecord{MediaID: item.ID, Files: []FileRecord{}}
		}
		status := record.Status
		if status != "archived" && status != "manual" {
			if item.MediaType == "MultiClipEdit" {
				status = "manual"
			} else {
				status = "pending"
			}
		}
		record.Status, record.SourcePresent, record.SourceSnapshot, record.Source = status, boolPointer(true), id, clean
		record.Filename, record.FileSize, record.CapturedAt, record.CreatedAt, record.MediaType = item.Filename, item.FileSize, item.CapturedAt, item.CreatedAt, item.MediaType
		if status == "manual" && record.Reason == "" {
			record.Reason = item.MediaType
			if record.Reason == "" {
				record.Reason = "manual export"
			}
		}
		a.Data.Items[item.ID] = record
		snapshotItems = append(snapshotItems, clean)
	}
	a.Data.Source = map[string]any{"provider": "gopro-cloud", "user_id": userID}
	a.Data.LatestSnapshot = id
	a.Data.Snapshots = append(a.Data.Snapshots, SnapshotRecord{ID: id, CapturedAt: nowUTC(), ItemCount: len(snapshotItems), Path: "snapshots/" + id + ".json.gz"})
	if err := os.MkdirAll(a.SnapshotsDir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(a.SnapshotsDir, id+".json.gz")
	temporary := target + ".tmp"
	file, err := os.Create(temporary)
	if err != nil {
		return "", err
	}
	zipper := gzip.NewWriter(file)
	err = json.NewEncoder(zipper).Encode(map[string]any{"schema_version": schemaVersion, "captured_at": nowUTC(), "user_id": userID, "items": snapshotItems})
	if closeErr := zipper.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporary, target)
	}
	if err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	return id, a.saveLocked()
}

func (a *Archive) Item(id string) *ItemRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Data.Items[id]
}

func (a *Archive) IsArchived(id string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	record := a.Data.Items[id]
	if record == nil || record.Status != "archived" || len(record.Files) == 0 {
		return false
	}
	for _, saved := range record.Files {
		path, err := secureJoin(a.Root, saved.Path)
		if err != nil {
			return false
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != saved.Size {
			return false
		}
	}
	return true
}

func (a *Archive) NeedsDownload(item MediaItem) bool {
	a.mu.RLock()
	record := a.Data.Items[item.ID]
	a.mu.RUnlock()
	if record != nil && record.Status == "manual" {
		return false
	}
	if record != nil && record.Integrity != nil && record.Integrity.Status == "failed" {
		return true
	}
	return !a.IsArchived(item.ID)
}

func (a *Archive) RecordDownload(item MediaItem, files []FileRecord, recovered string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	record := a.Data.Items[item.ID]
	if record == nil {
		record = &ItemRecord{MediaID: item.ID}
	}
	now := nowUTC()
	record.Status, record.SourcePresent, record.Filename, record.FileSize = "archived", boolPointer(true), item.Filename, item.FileSize
	record.CapturedAt, record.CreatedAt, record.MediaType, record.Files = item.CapturedAt, item.CreatedAt, item.MediaType, files
	if record.Source == nil {
		record.Source, _ = sanitizeSource(item.Raw).(map[string]any)
	}
	record.ArchivedAt, record.VerifiedAt, record.Error = now, now, ""
	record.Integrity = &IntegrityState{Status: "verified", VerifiedAt: now}
	if recovered != "" {
		record.ReplacedArtifacts = append(record.ReplacedArtifacts, ReplacedArtifact{Path: recovered, RetainedAt: now, Reason: "replaced during verified repair"})
	}
	a.Data.Items[item.ID] = record
	return a.saveLocked()
}

func (a *Archive) RecordFailure(id, message string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	record := a.Data.Items[id]
	if record == nil {
		record = &ItemRecord{MediaID: id, Files: []FileRecord{}}
		a.Data.Items[id] = record
	}
	record.Status, record.Error = "failed", message
	return a.saveLocked()
}

func (a *Archive) MarkManual(id, reason string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	record := a.Data.Items[id]
	if record == nil {
		return fmt.Errorf("unknown media ID: %s", id)
	}
	if record.Status == "archived" {
		return fmt.Errorf("cannot skip an archived item: %s", id)
	}
	record.Status, record.Reason = "manual", reason
	return a.saveLocked()
}

func (a *Archive) VerifyContext(ctx context.Context, root string) (VerificationResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if root == "" {
		root = a.Root
	}
	result := VerificationResult{}
	ids := make([]string, 0, len(a.Data.Items))
	for id := range a.Data.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		record := a.Data.Items[id]
		if record.Status != "archived" {
			continue
		}
		result.CheckedItems++
		if len(record.Files) == 0 {
			result.Issues = append(result.Issues, IntegrityIssue{MediaID: id, Kind: "empty", Message: "archived item has no files"})
			continue
		}
		for _, saved := range record.Files {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			result.CheckedFiles++
			path, err := secureJoin(root, saved.Path)
			if err != nil {
				result.Issues = append(result.Issues, IntegrityIssue{MediaID: id, Kind: "path", Message: err.Error(), Path: saved.Path})
				continue
			}
			info, err := os.Stat(path)
			if errors.Is(err, os.ErrNotExist) {
				result.Issues = append(result.Issues, IntegrityIssue{MediaID: id, Kind: "missing", Message: "file is missing", Path: saved.Path})
				continue
			}
			if err != nil || !info.Mode().IsRegular() {
				result.Issues = append(result.Issues, IntegrityIssue{MediaID: id, Kind: "read", Message: fmt.Sprint(err), Path: saved.Path})
				continue
			}
			result.CheckedBytes += info.Size()
			if info.Size() != saved.Size {
				result.Issues = append(result.Issues, IntegrityIssue{MediaID: id, Kind: "size", Message: fmt.Sprintf("expected %d bytes, found %d", saved.Size, info.Size()), Path: saved.Path})
				continue
			}
			digest, _, err := sha256FileContext(ctx, path)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return result, err
			}
			if err != nil {
				result.Issues = append(result.Issues, IntegrityIssue{MediaID: id, Kind: "read", Message: err.Error(), Path: saved.Path})
			} else if digest != saved.SHA256 {
				result.Issues = append(result.Issues, IntegrityIssue{MediaID: id, Kind: "checksum", Message: "SHA-256 does not match", Path: saved.Path})
			}
		}
	}
	return result, nil
}

func (a *Archive) Verify(root string) VerificationResult {
	result, _ := a.VerifyContext(context.Background(), root)
	return result
}

func (a *Archive) RecordVerification(result VerificationResult) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := nowUTC()
	byID := map[string][]IntegrityIssue{}
	for _, issue := range result.Issues {
		byID[issue.MediaID] = append(byID[issue.MediaID], issue)
	}
	changed := false
	for id, record := range a.Data.Items {
		if record.Status != "archived" {
			continue
		}
		changed = true
		if issues := byID[id]; len(issues) > 0 {
			record.Integrity = &IntegrityState{Status: "failed", CheckedAt: now, Issues: issues}
		} else {
			record.VerifiedAt = now
			record.Integrity = &IntegrityState{Status: "verified", VerifiedAt: now}
		}
	}
	if !changed {
		return nil
	}
	return a.saveLocked()
}

func (a *Archive) writeChecksumsLocked() error {
	lines := []string{}
	for _, item := range a.Data.Items {
		if item.Status == "archived" {
			for _, file := range item.Files {
				if file.SHA256 != "" && file.Path != "" {
					lines = append(lines, file.SHA256+"  "+file.Path)
				}
			}
		}
	}
	sort.Strings(lines)
	body := strings.Join(lines, "\n")
	if body != "" {
		body += "\n"
	}
	return atomicWrite(a.ChecksumsPath, []byte(body), 0o644)
}

func (a *Archive) Summary() Summary {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.summaryLocked()
}

func (a *Archive) summaryLocked() Summary {
	summary := Summary{}
	for _, item := range a.Data.Items {
		switch item.Status {
		case "archived":
			summary.Archived++
		case "manual":
			summary.Manual++
		}
		integrityFailed := item.Integrity != nil && item.Integrity.Status == "failed"
		if item.Status == "pending" || item.Status == "failed" || item.Status == "attention" || integrityFailed {
			summary.Blockers++
		}
		summary.ReplacedArtifacts += len(item.ReplacedArtifacts)
		for _, file := range item.Files {
			summary.Files++
			summary.Bytes += file.Size
		}
	}
	return summary
}
