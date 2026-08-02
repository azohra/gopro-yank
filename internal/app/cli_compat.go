package app

// This file keeps the pre-TUI command surface available for existing scripts.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

func listCommand(args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive root")
	pending := flags.Bool("pending", false, "only items needing attention")
	done := flags.Bool("done", false, "only successfully checked items")
	asJSON := flags.Bool("json", false, "JSON lines")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *pending && *done {
		return errors.New("--pending and --done are mutually exclusive")
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(archive.Data.Items))
	for id := range archive.Data.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		record := archive.Data.Items[id]
		bad := record.Integrity != nil && record.Integrity.Status == "failed"
		if *pending && record.Status == "archived" && !bad {
			continue
		}
		if *done && (record.Status != "archived" || bad) {
			continue
		}
		if *asJSON {
			payload, _ := json.Marshal(record)
			fmt.Println(string(payload))
		} else {
			fmt.Printf("%-10s %-10s %-24s %10s  %s\n", record.Status, firstDate(record.CapturedAt, record.CreatedAt), record.Filename, humanBytes(record.FileSize), record.MediaID)
		}
	}
	return nil
}

func firstDate(values ...string) string {
	for _, value := range values {
		if len(value) >= 10 {
			return value[:10]
		}
	}
	return "—"
}

func manifestCommand(args []string) error {
	flags := flag.NewFlagSet("manifest", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive root")
	outFile := flags.String("out-file", "", "write a copy")
	if err := flags.Parse(args); err != nil {
		return err
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(archive.Data, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if *outFile != "" {
		return atomicWrite(*outFile, payload, 0o644)
	}
	fmt.Print(string(payload))
	return nil
}

func reportCommand(args []string) error {
	flags := flag.NewFlagSet("report", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive root")
	open := flags.Bool("open", false, "open report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	path, err := renderReport(archive, nil)
	if err != nil {
		return err
	}
	fmt.Println(path)
	if *open {
		return openURL("file://" + filepath.ToSlash(path))
	}
	return nil
}

func skipCommand(args []string) error {
	flags := flag.NewFlagSet("skip", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive root")
	reason := flags.String("reason", "", "why this item needs manual handling")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *reason == "" || flags.NArg() == 0 {
		return errors.New("--reason and at least one media ID are required")
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	for _, id := range flags.Args() {
		if err := archive.MarkManual(id, *reason); err != nil {
			return err
		}
		fmt.Printf("manual %s · %s\n", id, *reason)
	}
	_, err = renderReport(archive, nil)
	return err
}

func demoCommand(args []string) error {
	flags := flag.NewFlagSet("demo", flag.ContinueOnError)
	count := flags.Int("count", 12, "simulated items")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *count < 1 {
		return errors.New("count must be positive")
	}
	total := int64(0)
	for index := 1; index <= *count; index++ {
		total += int64(128+index*17) * 1024 * 1024
	}
	fmt.Printf("GOPRO YANK / DEMO\nEvery original. Downloaded and verified.\n\nsource    %d simulated originals · %s\narchive   portable + resumable\n\n", *count, humanBytes(total))
	for index := 1; index <= *count; index++ {
		size := int64(128+index*17) * 1024 * 1024
		fmt.Printf("✓ %02d/%02d  demo-%03d  %9s  verified\n", index, *count, index, humanBytes(size))
		time.Sleep(90 * time.Millisecond)
	}
	fmt.Println("\nrecords   file list · checksums · report")
	fmt.Println("verdict   DOWNLOADABLE MEDIA EXPORT COMPLETE")
	return nil
}
