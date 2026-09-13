package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// LibraryInspection is a read-only view of the cloud library and local archive.
type LibraryInspection struct {
	Total          int
	TotalBytes     int64
	Archived       int
	ArchivedBytes  int64
	Remaining      int
	RemainingBytes int64
	Manual         int
	Earliest       string
	Latest         string
	Types          map[string]int
	ArchiveRoot    string
	items          []MediaItem
}

type ArchiveOptions struct {
	Root             string
	EnvPath          string
	LegacyState      string
	Parallel         int
	PerPage          int
	IgnoreSpaceCheck bool
}

type ArchiveEvent struct {
	Stage       string
	Current     int
	Total       int
	Transferred int64
	Result      *DownloadResult
	Inspection  *LibraryInspection
}

type ArchiveResult struct {
	Inspection   LibraryInspection
	Transferred  int64
	Failures     int
	Elapsed      time.Duration
	Verification VerificationResult
	ReportPath   string
	Summary      Summary
}

type VerifyResult struct {
	Verification VerificationResult
	Summary      Summary
	ReportPath   string
}

var ErrArchiveIncomplete = errors.New("archive needs attention")

func connectFromFile(ctx context.Context, envPath string) (*GoProClient, string, error) {
	token, user, err := loadCredentials(envPath)
	if err != nil {
		return nil, "", err
	}
	return connectGoPro(ctx, token, user)
}

func connectAccountInBrowser(ctx context.Context, envPath string) error {
	token, err := loginInBrowser(ctx)
	if err != nil {
		return fmt.Errorf("automatic sign-in failed: %w; try 'gopro-yank login --no-browser'", err)
	}
	_, user, err := connectGoPro(ctx, token, "")
	if err != nil {
		return err
	}
	return saveCredentials(envPath, token, user)
}

func inspectItems(items []MediaItem, archive *Archive) LibraryInspection {
	inspection := LibraryInspection{
		Total:       len(items),
		Types:       map[string]int{},
		ArchiveRoot: archive.Root,
		items:       append([]MediaItem(nil), items...),
	}
	for _, item := range items {
		inspection.TotalBytes += item.FileSize
		kind := strings.TrimSpace(item.MediaType)
		if kind == "" {
			kind = "Other"
		}
		inspection.Types[kind]++
		date := item.CapturedAt
		if date == "" {
			date = item.CreatedAt
		}
		if len(date) >= 10 {
			date = date[:10]
			if inspection.Earliest == "" || date < inspection.Earliest {
				inspection.Earliest = date
			}
			if inspection.Latest == "" || date > inspection.Latest {
				inspection.Latest = date
			}
		}
		if item.MediaType == "MultiClipEdit" {
			inspection.Manual++
			continue
		}
		if record := archive.Item(item.ID); record != nil && record.Status == "manual" {
			inspection.Manual++
			continue
		}
		if archive.IsArchived(item.ID) {
			inspection.Archived++
			inspection.ArchivedBytes += item.FileSize
			continue
		}
		inspection.Remaining++
		inspection.RemainingBytes += item.FileSize
	}
	return inspection
}

// InspectLibrary reads GoPro and the local archive without writing either one.
func InspectLibrary(ctx context.Context, root, envPath string, perPage int) (LibraryInspection, error) {
	if perPage < 1 || perPage > 100 {
		return LibraryInspection{}, errors.New("per-page must be 1–100")
	}
	client, _, err := connectFromFile(ctx, envPath)
	if err != nil {
		return LibraryInspection{}, err
	}
	items, err := client.ListAll(ctx, perPage)
	if err != nil {
		return LibraryInspection{}, err
	}
	archive, err := NewArchive(root)
	if err != nil {
		return LibraryInspection{}, err
	}
	return inspectItems(items, archive), nil
}

func ReplanLibrary(root string, inspection LibraryInspection) (LibraryInspection, error) {
	archive, err := NewArchive(root)
	if err != nil {
		return LibraryInspection{}, err
	}
	return inspectItems(inspection.items, archive), nil
}

func saveSourceSnapshot(ctx context.Context, client *GoProClient, archive *Archive, user string, perPage int, emit func(ArchiveEvent)) ([]MediaItem, error) {
	if emit != nil {
		emit(ArchiveEvent{Stage: "Reading your GoPro library"})
	}
	items, err := client.ListAll(ctx, perPage)
	if err != nil {
		return nil, err
	}
	if emit != nil {
		emit(ArchiveEvent{Stage: fmt.Sprintf("Saving details for %d originals", len(items)), Total: len(items)})
	}
	full, err := client.FullRecords(ctx, items, 4)
	if err != nil {
		return nil, err
	}
	_, err = archive.RecordSnapshot(items, user, full)
	return items, err
}

func validateArchiveOptions(options ArchiveOptions) error {
	if options.Root == "" {
		return errors.New("archive folder is required")
	}
	if options.Parallel < 1 || options.PerPage < 1 || options.PerPage > 100 {
		return errors.New("parallel must be positive and per-page must be 1–100")
	}
	return nil
}

func ArchiveLibrary(ctx context.Context, options ArchiveOptions, emit func(ArchiveEvent)) (ArchiveResult, error) {
	result := ArchiveResult{}
	if err := validateArchiveOptions(options); err != nil {
		return result, err
	}
	if emit == nil {
		emit = func(ArchiveEvent) {}
	}
	if err := os.MkdirAll(options.Root, 0o755); err != nil {
		return result, err
	}
	archive, err := NewArchive(options.Root)
	if err != nil {
		return result, err
	}
	if !archive.Exists && options.LegacyState != "" {
		if _, statErr := os.Stat(options.LegacyState); statErr == nil {
			emit(ArchiveEvent{Stage: "Checking your existing archive"})
			if _, err := archive.AdoptLegacy(options.LegacyState); err != nil {
				return result, err
			}
		}
	}
	client, user, err := connectFromFile(ctx, options.EnvPath)
	if err != nil {
		return result, err
	}
	items, err := saveSourceSnapshot(ctx, client, archive, user, options.PerPage, emit)
	if err != nil {
		return result, err
	}
	result.Inspection = inspectItems(items, archive)
	plan := result.Inspection
	emit(ArchiveEvent{Stage: "Ready to archive", Total: result.Inspection.Remaining, Inspection: &plan})

	todo := make([]MediaItem, 0, result.Inspection.Remaining)
	for _, item := range items {
		if archive.NeedsDownload(item) {
			todo = append(todo, item)
		}
	}
	if free, ok := freeDiskBytes(archive.Root); ok && !options.IgnoreSpaceCheck {
		overhead := int64(0)
		sizes := make([]int64, len(todo))
		for index, item := range todo {
			sizes[index] = item.FileSize
		}
		sort.Slice(sizes, func(i, j int) bool { return sizes[i] > sizes[j] })
		for index := 0; index < len(sizes) && index < options.Parallel; index++ {
			overhead += sizes[index]
		}
		if result.Inspection.RemainingBytes+overhead > free {
			return result, fmt.Errorf("not enough free space: %s available; downloads may need up to %s", humanBytes(free), humanBytes(result.Inspection.RemainingBytes+overhead))
		}
	}

	downloadContext, cancelDownloads := context.WithCancel(ctx)
	defer cancelDownloads()
	jobs := make(chan MediaItem)
	results := make(chan DownloadResult)
	var workers sync.WaitGroup
	started := time.Now()
	for range options.Parallel {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range jobs {
				download := downloadOne(downloadContext, client, item, archive, 5)
				if download.Status == "auth" {
					cancelDownloads()
				}
				results <- download
			}
		}()
	}
	go func() {
		for _, item := range todo {
			select {
			case jobs <- item:
			case <-downloadContext.Done():
				close(jobs)
				workers.Wait()
				close(results)
				return
			}
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	completed := 0
	for download := range results {
		completed++
		result.Transferred += download.Bytes
		if download.Status == "fail" || download.Status == "auth" {
			result.Failures++
		}
		copy := download
		emit(ArchiveEvent{Stage: "Archiving originals", Current: completed, Total: len(todo), Transferred: result.Transferred, Result: &copy})
	}
	result.Elapsed = time.Since(started)
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("stopped safely; finished files are saved: %w", err)
	}

	emit(ArchiveEvent{Stage: "Checking every archived file", Current: len(todo), Total: len(todo), Transferred: result.Transferred})
	result.Verification, err = archive.VerifyContext(ctx, "")
	if err != nil {
		return result, err
	}
	if err := archive.RecordVerification(result.Verification); err != nil {
		return result, err
	}
	result.ReportPath, err = renderReport(archive, &result.Verification)
	if err != nil {
		return result, err
	}
	result.Summary = archive.Summary()
	if result.Failures > 0 || !result.Verification.OK() {
		return result, ErrArchiveIncomplete
	}
	return result, nil
}

func VerifyArchive(ctx context.Context, root, replica string) (VerifyResult, error) {
	result := VerifyResult{}
	archive, err := requireArchive(root)
	if err != nil {
		return result, err
	}
	result.Verification, err = archive.VerifyContext(ctx, replica)
	if err != nil {
		return result, err
	}
	if replica == "" {
		if err := archive.RecordVerification(result.Verification); err != nil {
			return result, err
		}
		result.ReportPath, err = renderReport(archive, &result.Verification)
		if err != nil {
			return result, err
		}
	}
	result.Summary = archive.Summary()
	if !result.Verification.OK() || result.Summary.Blockers > 0 {
		return result, ErrArchiveIncomplete
	}
	return result, nil
}
