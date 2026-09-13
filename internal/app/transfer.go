package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type DownloadResult struct {
	MediaID, Status, Info string
	Bytes                 int64
}

func datePath(value string) string {
	if len(value) < 10 {
		return "_unsorted"
	}
	return value[:4] + "/" + value[5:7] + "/" + value[8:10]
}

func mediaSegment(id string) string { return "id-" + hex.EncodeToString([]byte(id)) }

func safeFilename(value string) string {
	base := filepath.Base(filepath.FromSlash(value))
	var builder strings.Builder
	for _, char := range base {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || strings.ContainsRune("-_.", char) {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), ".")
	if result == "" {
		result = "unknown"
	}
	stem := strings.ToUpper(strings.SplitN(result, ".", 2)[0])
	reserved := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
	for _, prefix := range []string{"COM", "LPT"} {
		for number := 1; number <= 9; number++ {
			reserved[fmt.Sprintf("%s%d", prefix, number)] = true
		}
	}
	if reserved[stem] {
		result = "_" + result
	}
	return result
}

func collisionKey(value string) string { return strings.ToLower(strings.TrimRight(value, " .")) }

func extractZIP(zipPath string, archive *Archive, item MediaItem) ([]FileRecord, string, error) {
	segment := mediaSegment(item.ID)
	captured := item.CapturedAt
	if captured == "" {
		captured = item.CreatedAt
	}
	relativeDir := filepath.ToSlash(filepath.Join("originals", filepath.FromSlash(datePath(captured)), segment))
	destination, err := secureJoin(archive.Root, relativeDir)
	if err != nil {
		return nil, "", err
	}
	staged := filepath.Join(archive.StagingDir, "extract", segment)
	if err := os.RemoveAll(staged); err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(staged, 0o755); err != nil {
		return nil, "", err
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", err
	}
	defer reader.Close()
	records := []FileRecord{}
	used := map[string]bool{}
	for index, member := range reader.File {
		if member.FileInfo().IsDir() {
			continue
		}
		original := safeFilename(member.Name)
		candidate := original
		suffix := index + 1
		if suffix < 2 {
			suffix = 2
		}
		for used[collisionKey(candidate)] {
			extension := filepath.Ext(original)
			stem := strings.TrimSuffix(original, extension)
			candidate = fmt.Sprintf("%s--part-%d%s", stem, suffix, extension)
			suffix++
		}
		used[collisionKey(candidate)] = true
		source, err := member.Open()
		if err != nil {
			return nil, "", err
		}
		target, err := os.OpenFile(filepath.Join(staged, candidate), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			source.Close()
			return nil, "", err
		}
		hash := sha256.New()
		size, copyErr := io.CopyBuffer(io.MultiWriter(target, hash), source, make([]byte, 4*1024*1024))
		if syncErr := target.Sync(); copyErr == nil {
			copyErr = syncErr
		}
		if closeErr := target.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if closeErr := source.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return nil, "", copyErr
		}
		if uint64(size) != member.UncompressedSize64 {
			return nil, "", fmt.Errorf("ZIP member size mismatch for %s", member.Name)
		}
		records = append(records, FileRecord{Path: filepath.ToSlash(filepath.Join(filepath.FromSlash(relativeDir), candidate)), Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)), ZIPCRC32: fmt.Sprintf("%08x", member.CRC32), ZIPMember: member.Name})
	}
	if len(records) == 0 {
		return nil, "", errors.New("source ZIP contains no files")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return nil, "", err
	}
	recovered := ""
	if _, err := os.Stat(destination); err == nil {
		matches := true
		entries, readErr := os.ReadDir(destination)
		if readErr != nil || len(entries) != len(records) {
			matches = false
		}
		if matches {
			for _, record := range records {
				path, _ := secureJoin(archive.Root, record.Path)
				digest, size, hashErr := sha256File(path)
				if hashErr != nil || size != record.Size || digest != record.SHA256 {
					matches = false
					break
				}
			}
		}
		if matches {
			if err := os.RemoveAll(staged); err != nil {
				return nil, "", err
			}
		} else {
			recovered = filepath.ToSlash(filepath.Join(controlName, "recovery", fmt.Sprintf("%s-%d", segment, time.Now().UnixNano())))
			recoveryPath, _ := secureJoin(archive.Root, recovered)
			if err := os.MkdirAll(filepath.Dir(recoveryPath), 0o755); err != nil {
				return nil, "", err
			}
			if err := os.Rename(destination, recoveryPath); err != nil {
				return nil, "", err
			}
			if err := os.Rename(staged, destination); err != nil {
				return nil, "", err
			}
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(staged, destination); err != nil {
			return nil, "", err
		}
	} else {
		return nil, "", err
	}
	return records, recovered, nil
}

func safeError(err error, token string) string {
	message := err.Error()
	if token != "" {
		message = strings.ReplaceAll(message, token, "[redacted]")
		message = strings.ReplaceAll(message, url.QueryEscape(token), "[redacted]")
	}
	return message
}

func downloadOne(ctx context.Context, client *GoProClient, item MediaItem, archive *Archive, attempts int) DownloadResult {
	if !archive.NeedsDownload(item) {
		return DownloadResult{MediaID: item.ID, Status: "skip", Info: "archive record is present"}
	}
	if err := os.MkdirAll(archive.StagingDir, 0o755); err != nil {
		return DownloadResult{MediaID: item.ID, Status: "fail", Info: err.Error()}
	}
	temporary := filepath.Join(archive.StagingDir, mediaSegment(item.ID)+".zip.part")
	var last string
	for attempt := 0; attempt < attempts; attempt++ {
		file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		var written int64
		if err == nil {
			written, err = client.StreamSourceZIP(ctx, item.ID, file)
			if syncErr := file.Sync(); err == nil {
				err = syncErr
			}
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
		}
		if err == nil {
			var files []FileRecord
			var recovered string
			files, recovered, err = extractZIP(temporary, archive, item)
			if err == nil {
				_ = os.Remove(temporary)
				if err = archive.RecordDownload(item, files, recovered); err == nil {
					return DownloadResult{MediaID: item.ID, Status: "ok", Bytes: written}
				}
			}
		}
		_ = os.Remove(temporary)
		if errors.Is(err, ErrAuth) {
			return DownloadResult{MediaID: item.ID, Status: "auth", Info: safeError(err, client.Token)}
		}
		last = safeError(err, client.Token)
		if attempt+1 < attempts {
			select {
			case <-ctx.Done():
				last = ctx.Err().Error()
				attempt = attempts
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
	}
	_ = archive.RecordFailure(item.ID, last)
	return DownloadResult{MediaID: item.ID, Status: "fail", Info: last}
}
