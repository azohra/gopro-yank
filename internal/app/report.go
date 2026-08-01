package app

import (
	"fmt"
	"html/template"
	"sort"
	"strings"
)

func humanBytes(value int64) string {
	size := float64(value)
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	for _, unit := range units {
		if size < 1024 || unit == "PB" {
			return fmt.Sprintf("%.1f %s", size, unit)
		}
		size /= 1024
	}
	return fmt.Sprintf("%.1f PB", size)
}

type reportRow struct{ ID, Filename, Status, Reason string }
type reportGroup struct {
	Name  string
	Items int
	Bytes int64
}
type reportData struct {
	Verdict, VerdictClass, Snapshot, Integrity string
	Summary                                    Summary
	Years                                      []reportGroup
	Types                                      []reportGroup
	Attention                                  []reportRow
	Issues                                     []IntegrityIssue
}

const reportTemplate = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>GoPro Yank archive report</title><style>
:root{color-scheme:dark;--bg:#0b1115;--panel:#111c22;--line:#30424b;--text:#f4f0e8;--muted:#8fa7b2;--yank:#ff5c35;--proof:#58e0b4;--amber:#f2bd5b}*{box-sizing:border-box}body{margin:0;font:15px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--bg);color:var(--text)}main{width:min(1100px,calc(100% - 32px));margin:48px auto 80px}h1{font-size:clamp(2.2rem,7vw,4.8rem);letter-spacing:-.04em;margin:.2rem 0}.muted{color:var(--muted)}.verdict{margin:28px 0;padding:22px;border:1px solid var(--line);border-left:6px solid var(--amber);background:var(--panel);border-radius:10px}.verdict.good{border-left-color:var(--proof)}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px;margin:24px 0}.metric,.card{border:1px solid var(--line);background:var(--panel);border-radius:10px;padding:18px}.metric strong{display:block;font-size:1.65rem;color:var(--proof)}.two{display:grid;grid-template-columns:repeat(auto-fit,minmax(310px,1fr));gap:16px;margin:16px 0}table{width:100%;border-collapse:collapse}th,td{padding:9px 8px;text-align:left;border-bottom:1px solid var(--line);vertical-align:top}th{color:var(--muted)}code{color:var(--yank);word-break:break-all}.status{color:var(--amber)}footer{margin-top:32px;color:var(--muted)}</style></head><body><main>
<div class="muted">GOPRO YANK / VERIFIED ARCHIVE</div><h1>Home + accounted for.</h1><p class="muted">Portable proof generated from <code>.gopro-yank/manifest.json</code>.</p>
<section class="verdict {{.VerdictClass}}"><strong>{{.Verdict}}</strong><br>Source snapshot: {{.Snapshot}} · Integrity: {{.Integrity}}</section>
<section class="grid"><div class="metric"><strong>{{.Summary.Archived}}</strong>archived items</div><div class="metric"><strong>{{.Summary.Files}}</strong>original files</div><div class="metric"><strong>{{human .Summary.Bytes}}</strong>preserved</div><div class="metric"><strong>{{.Summary.Manual}}</strong>manual items</div><div class="metric"><strong>{{.Summary.Blockers}}</strong>archive blockers</div><div class="metric"><strong>{{.Summary.ReplacedArtifacts}}</strong>retained repairs</div></section>
<section class="two"><div class="card"><h2>By year</h2><table><tr><th>Year</th><th>Items</th><th>Size</th></tr>{{range .Years}}<tr><td>{{.Name}}</td><td>{{.Items}}</td><td>{{human .Bytes}}</td></tr>{{end}}</table></div><div class="card"><h2>By media type</h2><table><tr><th>Type</th><th>Items</th></tr>{{range .Types}}<tr><td>{{.Name}}</td><td>{{.Items}}</td></tr>{{end}}</table></div></section>
<section class="card"><h2>Needs attention</h2><table><tr><th>ID</th><th>Name</th><th>Status</th><th>Reason</th></tr>{{range .Attention}}<tr><td><code>{{.ID}}</code></td><td>{{.Filename}}</td><td class="status">{{.Status}}</td><td>{{.Reason}}</td></tr>{{else}}<tr><td colspan="4" class="muted">No archive blockers.</td></tr>{{end}}</table></section>
<section class="card" style="margin-top:16px"><h2>Integrity</h2><table><tr><th>ID</th><th>Issue</th><th>Path</th><th>Detail</th></tr>{{range .Issues}}<tr><td><code>{{.MediaID}}</code></td><td>{{.Kind}}</td><td>{{.Path}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="4" class="muted">No recorded integrity issues.</td></tr>{{end}}</table></section>
<footer>Schema 1. This report proves the recorded media export; it does not evaluate other GoPro subscription benefits.</footer></main></body></html>`

func renderReport(archive *Archive, verification *VerificationResult) (string, error) {
	archive.mu.RLock()
	defer archive.mu.RUnlock()
	summary := archive.summaryLocked()
	years := map[string]*reportGroup{}
	types := map[string]*reportGroup{}
	attention := []reportRow{}
	for _, item := range archive.Data.Items {
		date := item.CapturedAt
		if date == "" {
			date = item.CreatedAt
		}
		year := "Unknown"
		if len(date) >= 4 {
			year = date[:4]
		}
		if years[year] == nil {
			years[year] = &reportGroup{Name: year}
		}
		years[year].Items++
		itemBytes := int64(0)
		for _, file := range item.Files {
			itemBytes += file.Size
		}
		years[year].Bytes += itemBytes
		kind := item.MediaType
		if kind == "" {
			kind = "Unknown"
		}
		if types[kind] == nil {
			types[kind] = &reportGroup{Name: kind}
		}
		types[kind].Items++
		if item.Status != "archived" || (item.Integrity != nil && item.Integrity.Status == "failed") {
			reason := item.Reason
			if reason == "" {
				reason = item.Error
			}
			if reason == "" && item.Integrity != nil && len(item.Integrity.Issues) > 0 {
				reason = item.Integrity.Issues[0].Message
			}
			attention = append(attention, reportRow{item.MediaID, item.Filename, item.Status, reason})
		}
	}
	toSlice := func(values map[string]*reportGroup) []reportGroup {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make([]reportGroup, 0, len(keys))
		for _, key := range keys {
			result = append(result, *values[key])
		}
		return result
	}
	verdict, class := "DOWNLOADABLE MEDIA EXPORT COMPLETE", "good"
	if summary.Blockers > 0 {
		verdict, class = "MEDIA EXPORT NEEDS ATTENTION", "warn"
	}
	integrity := "Last recorded state"
	issues := []IntegrityIssue{}
	if verification != nil {
		issues = verification.Issues
		if verification.OK() {
			integrity = "Verified"
		} else {
			integrity = fmt.Sprintf("%d issue(s)", len(issues))
			verdict, class = "MEDIA EXPORT NEEDS ATTENTION", "warn"
		}
	}
	data := reportData{Verdict: verdict, VerdictClass: class, Snapshot: archive.Data.LatestSnapshot, Integrity: integrity, Summary: summary, Years: toSlice(years), Types: toSlice(types), Attention: attention, Issues: issues}
	tmpl, err := template.New("report").Funcs(template.FuncMap{"human": humanBytes}).Parse(reportTemplate)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	if err := tmpl.Execute(&output, data); err != nil {
		return "", err
	}
	return archive.ReportPath, atomicWrite(archive.ReportPath, []byte(output.String()), 0o644)
}
