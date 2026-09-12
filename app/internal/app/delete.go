package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type DeletePlan struct {
	Root      string
	Originals int
	Files     int
	Bytes     int64
}

type DeleteResult struct {
	RemovedFiles int
	RemovedBytes int64
}

func PlanLocalArchive(root string) (DeletePlan, error) {
	archive, err := NewArchive(root)
	if err != nil {
		return DeletePlan{}, err
	}
	if !archive.Exists {
		return DeletePlan{}, errors.New("no GoPro Yank archive was found in that folder")
	}
	summary := archive.Summary()
	return DeletePlan{
		Root:      archive.Root,
		Originals: summary.Archived,
		Files:     summary.Files,
		Bytes:     summary.Bytes,
	}, nil
}

func DeleteLocalArchive(ctx context.Context, root string) (DeleteResult, error) {
	archive, err := NewArchive(root)
	if err != nil {
		return DeleteResult{}, err
	}
	if !archive.Exists {
		return DeleteResult{}, errors.New("no GoPro Yank archive was found in that folder")
	}
	return archive.deleteLocal(ctx)
}

func (a *Archive) deleteLocal(ctx context.Context) (DeleteResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	controlDir, err := secureJoin(a.Root, controlName)
	if err != nil {
		return DeleteResult{}, fmt.Errorf("refusing to delete an unsafe archive: %w", err)
	}

	targets := make([]string, 0)
	seen := map[string]bool{}
	for _, item := range a.Data.Items {
		for _, file := range item.Files {
			target, err := secureJoin(a.Root, file.Path)
			if err != nil {
				return DeleteResult{}, fmt.Errorf("refusing to delete an unsafe archive: %w", err)
			}
			if !seen[target] {
				seen[target] = true
				targets = append(targets, target)
			}
		}
	}
	sort.Strings(targets)

	result := DeleteResult{}
	directories := map[string]bool{}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		info, statErr := os.Lstat(target)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return result, fmt.Errorf("inspect %s: %w", target, statErr)
		}
		if info.IsDir() {
			return result, fmt.Errorf("refusing to delete a directory recorded as a file: %s", target)
		}
		if err := os.Remove(target); err != nil {
			return result, fmt.Errorf("delete %s: %w", target, err)
		}
		result.RemovedFiles++
		result.RemovedBytes += info.Size()
		for parent := filepath.Dir(target); parent != a.Root && parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			directories[parent] = true
		}
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := os.RemoveAll(controlDir); err != nil {
		return result, fmt.Errorf("delete GoPro Yank records: %w", err)
	}

	emptyDirectories := make([]string, 0, len(directories))
	for directory := range directories {
		emptyDirectories = append(emptyDirectories, directory)
	}
	sort.Slice(emptyDirectories, func(i, j int) bool {
		return strings.Count(emptyDirectories[i], string(filepath.Separator)) > strings.Count(emptyDirectories[j], string(filepath.Separator))
	})
	for _, directory := range emptyDirectories {
		_ = os.Remove(directory)
	}

	a.Exists = false
	return result, nil
}
