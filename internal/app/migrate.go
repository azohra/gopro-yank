package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type legacyMarker struct {
	MediaID, Status, Filename, CreatedAt, Reason string
	FileSize                                     int64
	Saved                                        []string
}

func readLegacyMarker(path string) (legacyMarker, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return legacyMarker{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return legacyMarker{}, err
	}
	id := stringValue(raw["media_id"])
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	saved := []string{}
	if values, ok := raw["saved"].([]any); ok {
		for _, value := range values {
			saved = append(saved, stringValue(value))
		}
	}
	return legacyMarker{MediaID: id, Status: stringValue(raw["status"]), Filename: stringValue(raw["filename"]), CreatedAt: stringValue(raw["created_at"]), Reason: stringValue(raw["reason"]), FileSize: int64Value(raw["file_size"]), Saved: saved}, nil
}

func (a *Archive) AdoptLegacy(stateDir string) (AdoptionResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := AdoptionResult{}
	if a.Exists {
		return result, nil
	}
	paths, err := filepath.Glob(filepath.Join(stateDir, "*.json"))
	if err != nil {
		return result, err
	}
	markers := []legacyMarker{}
	owners := map[string][]string{}
	for _, path := range paths {
		marker, err := readLegacyMarker(path)
		if err != nil {
			continue
		}
		markers = append(markers, marker)
		for _, saved := range marker.Saved {
			owners[saved] = append(owners[saved], marker.MediaID)
		}
	}
	for _, marker := range markers {
		record := &ItemRecord{MediaID: marker.MediaID, SourcePresent: nil, Filename: marker.Filename, FileSize: marker.FileSize, CreatedAt: marker.CreatedAt, CapturedAt: marker.CreatedAt, Files: []FileRecord{}, AdoptedAt: nowUTC(), Source: map[string]any{"id": marker.MediaID, "filename": marker.Filename, "file_size": marker.FileSize, "created_at": marker.CreatedAt, "_legacy_marker": true}}
		if marker.Status == "skipped" {
			record.Status = "manual"
			record.Reason = marker.Reason
			if record.Reason == "" {
				record.Reason = "legacy skip"
			}
			result.ManualItems++
			a.Data.Items[marker.MediaID] = record
			continue
		}
		reasons := []string{}
		if len(marker.Saved) == 0 {
			reasons = append(reasons, "legacy marker has no saved files")
		}
		for _, saved := range marker.Saved {
			if len(owners[saved]) > 1 {
				reasons = append(reasons, "shared legacy path: "+saved)
				continue
			}
			path, pathErr := secureJoin(a.Root, saved)
			if pathErr != nil {
				reasons = append(reasons, "unsafe legacy path: "+saved)
				continue
			}
			info, statErr := os.Stat(path)
			if errors.Is(statErr, os.ErrNotExist) {
				reasons = append(reasons, "missing legacy path: "+saved)
				continue
			}
			if statErr != nil || !info.Mode().IsRegular() {
				reasons = append(reasons, "unreadable legacy path: "+saved)
			}
		}
		if len(reasons) > 0 {
			record.Status = "attention"
			record.Reason = strings.Join(reasons, "; ")
			result.AttentionItems++
			a.Data.Items[marker.MediaID] = record
			continue
		}
		for _, saved := range marker.Saved {
			path, _ := secureJoin(a.Root, saved)
			digest, size, err := sha256File(path)
			if err != nil {
				return result, fmt.Errorf("hash legacy file %s: %w", saved, err)
			}
			record.Files = append(record.Files, FileRecord{Path: filepath.ToSlash(saved), Size: size, SHA256: digest, ZIPMember: filepath.Base(saved), Adopted: true})
			result.AdoptedFiles++
			result.BytesHashed += size
		}
		now := nowUTC()
		record.Status = "archived"
		record.VerifiedAt = now
		record.Integrity = &IntegrityState{Status: "verified", VerifiedAt: now}
		result.AdoptedItems++
		a.Data.Items[marker.MediaID] = record
	}
	if len(markers) > 0 {
		a.Data.LegacyAdoption = map[string]any{"adopted_at": nowUTC(), "state_dir": stateDir, "marker_count": len(markers)}
		if err := a.saveLocked(); err != nil {
			return result, err
		}
	}
	return result, nil
}
