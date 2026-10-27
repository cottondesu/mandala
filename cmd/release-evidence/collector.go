package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Command statuses. INTERRUPTED and RUNNING are never PASS; NOT_RUN is the
// initial state and the state of commands skipped by fail-fast or preflight.
const (
	StatusPASS        = "PASS"
	StatusFAIL        = "FAIL"
	StatusNOTRUN      = "NOT_RUN"
	StatusRUNNING     = "RUNNING"
	StatusINTERRUPTED = "INTERRUPTED"
	StatusLAUNCHERROR = "LAUNCH_ERROR"
	StatusBLOCKED     = "BLOCKED"
)

type commandSpec struct {
	slug string
	argv []string // argv[0]=="go" is replaced by the selected -go path
}

// gateCommands is the fixed, whitelisted LOCAL_TESTS_VALIDATED command set
// (docs/RELEASING.md Step 2). Nothing else is ever executed by the collector.
var gateCommands = []commandSpec{
	{"go-test-count1", []string{"go", "test", "-count=1", "./..."}},
	{"go-test-shuffle", []string{"go", "test", "-shuffle=on", "-count=1", "./..."}},
	{"go-test-race-shuffle", []string{"go", "test", "-race", "-shuffle=on", "-count=1", "./..."}},
	{"go-vet", []string{"go", "vet", "./..."}},
	{"go-build", []string{"go", "build", "./..."}},
	{"git-diff-check", []string{"git", "diff", "--check"}},
}

var disclaimers = []string{
	"Local evidence input for the LOCAL_TESTS_VALIDATED gate only.",
	"Does not assert REMOTE_CI_VALIDATED (remote CI on the exact candidate SHA).",
	"Does not assert Windows results; run the same gates on Windows separately.",
	"Does not assert INDEPENDENT_REVIEW_VALIDATED or RELEASE_APPROVED.",
	"Tagging, publication, and approval remain separate human steps per docs/RELEASING.md.",
}

type SourceSnapshot struct {
	CapturedUTC      string `json:"captured_utc"`
	HeadSHA          string `json:"head_sha"`
	HeadQueryError   string `json:"head_query_error,omitempty"`
	StatusPorcelain  string `json:"status_porcelain"`
	StatusQueryError string `json:"status_query_error,omitempty"`
	Dirty            bool   `json:"dirty"`
}

type ToolchainInfo struct {
	GoPath        string   `json:"go_path"`
	VersionArgv   []string `json:"version_argv"`
	VersionOutput string   `json:"version_output"`
	VersionError  string   `json:"version_error,omitempty"`
	EnvArgv       []string `json:"env_argv"`
	EnvOutput     string   `json:"env_output"`
	EnvError      string   `json:"env_error,omitempty"`
	GOVersion     string   `json:"goversion"`
	GOOS          string   `json:"goos"`
	GOARCH        string   `json:"goarch"`
	ExpectedMinor string   `json:"expected_go_minor,omitempty"`
	ActualMinor   string   `json:"actual_minor"`
	MinorMatches  *bool    `json:"minor_matches,omitempty"`
}

type CommandRecord struct {
	ID           int      `json:"id"`
	Slug         string   `json:"slug"`
	Argv         []string `json:"argv"`
	Status       string   `json:"status"`
	ExitCode     *int     `json:"exit_code"` // null when the command did not really run
	LaunchError  string   `json:"launch_error,omitempty"`
	WaitError    string   `json:"wait_error,omitempty"`
	StartUTC     string   `json:"start_utc,omitempty"`
	EndUTC       string   `json:"end_utc,omitempty"`
	StdoutFile   string   `json:"stdout_file,omitempty"`
	StdoutSHA256 string   `json:"stdout_sha256,omitempty"`
	StdoutBytes  int      `json:"stdout_bytes,omitempty"`
	StderrFile   string   `json:"stderr_file,omitempty"`
	StderrSHA256 string   `json:"stderr_sha256,omitempty"`
	StderrBytes  int      `json:"stderr_bytes,omitempty"`
}

type Manifest struct {
	SchemaVersion         int              `json:"schema_version"`
	Gate                  string           `json:"gate"`
	CollectorArgv         []string         `json:"collector_argv"`
	StartedUTC            string           `json:"started_utc"`
	FinishedUTC           string           `json:"finished_utc,omitempty"`
	RepoRoot              string           `json:"repo_root"`
	OutputDir             string           `json:"output_dir"`
	CandidateSHA          string           `json:"candidate_sha"`
	AllowDirty            bool             `json:"allow_dirty"`
	SourceInitial         *SourceSnapshot  `json:"source_initial"`
	SourceFinal           *SourceSnapshot  `json:"source_final"`
	ClaimedExactCandidate bool             `json:"claimed_exact_candidate"`
	Toolchain             *ToolchainInfo   `json:"toolchain"`
	Commands              []*CommandRecord `json:"commands"`
	Queries               []*CommandRecord `json:"queries"`
	Interrupted           bool             `json:"interrupted"`
	ReleaseStatus         string           `json:"release_status"` // PASS only when everything below supports it
	BlockedReasons        []string         `json:"blocked_reasons"`
	Disclaimers           []string         `json:"disclaimers"`
}

// Collector owns one evidence run.
type Collector struct {
	RepoRoot, OutputDir, Candidate, GoPath, ExpectedMinor string
	AllowDirty                                            bool
	Runner                                                Runner
	Ctx                                                   context.Context
	StartedUTC                                            string

	m *Manifest
}

func (c *Collector) Manifest() *Manifest { return c.m }

func (c *Collector) Collect() error {
	m := &Manifest{
		SchemaVersion: 1,
		Gate:          "LOCAL_TESTS_VALIDATED",
		CollectorArgv: os.Args,
		StartedUTC:    c.StartedUTC,
		RepoRoot:      c.RepoRoot,
		OutputDir:     c.OutputDir,
		CandidateSHA:  c.Candidate,
		AllowDirty:    c.AllowDirty,
		ReleaseStatus: StatusRUNNING,
		Disclaimers:   disclaimers,
	}
	for i, spec := range gateCommands {
		m.Commands = append(m.Commands, &CommandRecord{ID: i + 1, Slug: spec.slug, Argv: spec.argv, Status: StatusNOTRUN})
	}
	c.m = m
	if err := c.persist(); err != nil {
		return err
	}

	var err error
	m.SourceInitial, err = c.captureSource("initial")
	if err != nil {
		return err
	}
	m.Toolchain, err = c.captureToolchain()
	if err != nil {
		return err
	}
	hard, soft := c.preflight()
	m.BlockedReasons = append(append([]string{}, hard...), soft...)
	if c.Ctx.Err() != nil {
		m.Interrupted = true
	}
	if err := c.persist(); err != nil {
		return err
	}

	if len(hard) == 0 && !m.Interrupted {
		if err := c.runGateCommands(); err != nil {
			return err
		}
	}
	// The final source check is collected even after command failures.
	m.SourceFinal, err = c.captureSource("final")
	if err != nil {
		return err
	}
	if c.Ctx.Err() != nil {
		m.Interrupted = true
	}
	c.finalize(hard, soft)
	if err := c.persist(); err != nil {
		return err
	}
	return nil
}

func (c *Collector) runGateCommands() error {
	for i, spec := range gateCommands {
		if c.Ctx.Err() != nil {
			c.m.Interrupted = true
			return nil // remaining commands stay NOT_RUN
		}
		rec := c.m.Commands[i]
		argv := append([]string{}, spec.argv...)
		if argv[0] == "go" {
			argv[0] = c.GoPath
		}
		rec.Argv, rec.Status = argv, StatusRUNNING
		if err := c.persist(); err != nil { // persisted before launch: hard kill leaves RUNNING
			return err
		}
		res := c.Runner.Run(argv)
		if err := c.fillRecord(rec, res); err != nil {
			return err
		}
		if err := c.persist(); err != nil {
			return err
		}
		if res.Interrupted {
			c.m.Interrupted = true
			return nil
		}
		if res.LaunchError != "" || res.WaitError != "" || res.ExitCode != 0 {
			return nil // fail-fast: later commands stay NOT_RUN
		}
	}
	return nil
}

func (c *Collector) fillRecord(rec *CommandRecord, res ExecResult) error {
	rec.StartUTC, rec.EndUTC = fmtTime(res.Start), fmtTime(res.End)
	if res.LaunchError != "" {
		rec.Status, rec.LaunchError = StatusLAUNCHERROR, res.LaunchError
		return nil
	}
	code := res.ExitCode
	rec.ExitCode = &code
	rec.WaitError = res.WaitError
	var err error
	if rec.StdoutFile, rec.StdoutSHA256, rec.StdoutBytes, err = writeCapture(c.OutputDir, rec, ".stdout", res.Stdout); err != nil {
		return err
	}
	if rec.StderrFile, rec.StderrSHA256, rec.StderrBytes, err = writeCapture(c.OutputDir, rec, ".stderr", res.Stderr); err != nil {
		return err
	}
	switch {
	case res.Interrupted:
		rec.Status = StatusINTERRUPTED
	case res.ExitCode == 0 && res.WaitError == "":
		rec.Status = StatusPASS
	default:
		rec.Status = StatusFAIL
	}
	return nil
}

func writeCapture(dir string, rec *CommandRecord, ext string, b []byte) (file, sha string, size int, err error) {
	name := fmt.Sprintf("%02d-%s%s", rec.ID, rec.Slug, ext)
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		return "", "", 0, err
	}
	sum := sha256.Sum256(b)
	return name, hex.EncodeToString(sum[:]), len(b), nil
}

func (c *Collector) query(slug string, argv []string) (ExecResult, error) {
	rec := &CommandRecord{ID: len(gateCommands) + len(c.m.Queries) + 1, Slug: slug, Argv: argv, Status: StatusRUNNING}
	c.m.Queries = append(c.m.Queries, rec)
	if err := c.persist(); err != nil {
		return ExecResult{}, err
	}
	res := c.Runner.Run(argv)
	if err := c.fillRecord(rec, res); err != nil {
		return res, err
	}
	if err := c.persist(); err != nil {
		return res, err
	}
	return res, nil
}

func (c *Collector) captureSource(stage string) (*SourceSnapshot, error) {
	s := &SourceSnapshot{CapturedUTC: nowUTC()}
	head, err := c.query(stage+"-head", []string{"git", "rev-parse", "HEAD"})
	if err != nil {
		return nil, err
	}
	switch {
	case head.LaunchError != "":
		s.HeadQueryError = "launch error: " + head.LaunchError
	case head.WaitError != "":
		s.HeadQueryError = "wait error: " + head.WaitError
	case head.ExitCode != 0:
		s.HeadQueryError = fmt.Sprintf("exit %d: %s", head.ExitCode, strings.TrimSpace(string(head.Stderr)))
	default:
		s.HeadSHA = strings.TrimSpace(string(head.Stdout))
	}
	st, err := c.query(stage+"-status", []string{"git", "status", "--porcelain", "--untracked-files=all"})
	if err != nil {
		return nil, err
	}
	switch {
	case st.LaunchError != "":
		s.StatusQueryError = "launch error: " + st.LaunchError
	case st.WaitError != "":
		s.StatusQueryError = "wait error: " + st.WaitError
	case st.ExitCode != 0:
		s.StatusQueryError = fmt.Sprintf("exit %d: %s", st.ExitCode, strings.TrimSpace(string(st.Stderr)))
	default:
		s.StatusPorcelain = string(st.Stdout)
		s.Dirty = strings.TrimSpace(s.StatusPorcelain) != ""
	}
	if head.Interrupted || st.Interrupted {
		c.m.Interrupted = true
	}
	return s, nil
}

func (c *Collector) captureToolchain() (*ToolchainInfo, error) {
	t := &ToolchainInfo{GoPath: c.GoPath, ExpectedMinor: c.ExpectedMinor}
	t.VersionArgv = []string{c.GoPath, "version"}
	v, err := c.query("go-version", t.VersionArgv)
	if err != nil {
		return nil, err
	}
	t.VersionOutput = string(v.Stdout)
	t.VersionError = describeError(v)
	t.EnvArgv = []string{c.GoPath, "env", "GOVERSION", "GOOS", "GOARCH"}
	e, err := c.query("go-env", t.EnvArgv)
	if err != nil {
		return nil, err
	}
	t.EnvOutput = string(e.Stdout)
	t.EnvError = describeError(e)
	if fields := strings.Fields(t.EnvOutput); len(fields) == 3 && t.EnvError == "" {
		t.GOVersion, t.GOOS, t.GOARCH = fields[0], fields[1], fields[2]
	}
	t.ActualMinor = minorOfGOVERSION(t.GOVersion)
	if c.ExpectedMinor != "" && t.VersionError == "" && t.EnvError == "" {
		matches := t.ActualMinor == c.ExpectedMinor
		t.MinorMatches = &matches
	}
	if v.Interrupted || e.Interrupted {
		c.m.Interrupted = true
	}
	return t, nil
}

func describeError(r ExecResult) string {
	switch {
	case r.LaunchError != "":
		return "launch error: " + r.LaunchError
	case r.WaitError != "":
		return "wait error: " + r.WaitError
	case r.ExitCode != 0:
		return fmt.Sprintf("exit %d: %s", r.ExitCode, strings.TrimSpace(string(r.Stderr)))
	}
	return ""
}

// preflight splits blocking findings into hard (gate commands must not run)
// and soft (commands may run, but the release status stays BLOCKED).
func (c *Collector) preflight() (hard, soft []string) {
	si, t := c.m.SourceInitial, c.m.Toolchain
	if si.HeadQueryError != "" {
		hard = append(hard, "initial git rev-parse HEAD failed: "+si.HeadQueryError)
	}
	if si.StatusQueryError != "" {
		hard = append(hard, "initial git status failed: "+si.StatusQueryError)
	}
	if si.HeadQueryError == "" && si.HeadSHA != c.Candidate {
		hard = append(hard, fmt.Sprintf("initial HEAD %q is not the expected candidate %q; evidence cannot be attributed to the candidate", si.HeadSHA, c.Candidate))
	}
	if si.StatusQueryError == "" && si.Dirty {
		if c.AllowDirty {
			soft = append(soft, "source tree is dirty; -allow-dirty collects development evidence only: release status BLOCKED and no exact-candidate claim")
		} else {
			hard = append(hard, "source tree is dirty (git status --porcelain --untracked-files=all is non-empty); rerun from a clean checkout, or pass -allow-dirty for development-only BLOCKED evidence")
		}
	}
	if t.VersionError != "" {
		hard = append(hard, "go version query failed: "+t.VersionError)
	}
	if t.EnvError != "" {
		hard = append(hard, "go env GOVERSION GOOS GOARCH query failed: "+t.EnvError)
	}
	if c.ExpectedMinor == "" {
		hard = append(hard, "-expected-go-minor is required for a PASS")
	}
	if t.EnvError == "" && (t.ActualMinor == "" || t.GOOS == "" || t.GOARCH == "") {
		hard = append(hard, "go env returned malformed GOVERSION GOOS GOARCH")
	}
	if t.VersionError == "" && t.EnvError == "" {
		fields := strings.Fields(t.VersionOutput)
		if len(fields) < 4 || fields[0] != "go" || fields[1] != "version" || fields[2] != t.GOVersion || fields[3] != t.GOOS+"/"+t.GOARCH {
			hard = append(hard, "go version and go env are malformed or disagree")
		}
	}
	if c.ExpectedMinor != "" && t.VersionError == "" && t.EnvError == "" {
		if !*t.MinorMatches {
			hard = append(hard, fmt.Sprintf("Go toolchain minor %q does not match -expected-go-minor %q", t.ActualMinor, c.ExpectedMinor))
		}
	}
	return hard, soft
}

func (c *Collector) finalize(hard, soft []string) {
	m, si, sf := c.m, c.m.SourceInitial, c.m.SourceFinal
	m.ClaimedExactCandidate = si.HeadSHA != "" && si.HeadSHA == m.CandidateSHA &&
		!si.Dirty && si.HeadQueryError == "" && si.StatusQueryError == "" && sf != nil &&
		sf.HeadQueryError == "" && sf.StatusQueryError == "" && !sf.Dirty && sf.HeadSHA == m.CandidateSHA
	reasons := append(append([]string{}, hard...), soft...)
	if sf != nil {
		if sf.HeadQueryError != "" {
			reasons = append(reasons, "final git rev-parse HEAD failed: "+sf.HeadQueryError)
		}
		if sf.StatusQueryError != "" {
			reasons = append(reasons, "final git status failed: "+sf.StatusQueryError)
		}
		if sf.HeadQueryError == "" && si.HeadSHA != sf.HeadSHA {
			reasons = append(reasons, fmt.Sprintf("HEAD changed during collection: %q -> %q", si.HeadSHA, sf.HeadSHA))
		}
		if sf.HeadQueryError == "" && sf.HeadSHA != m.CandidateSHA {
			reasons = append(reasons, fmt.Sprintf("final HEAD %q is not the candidate %q", sf.HeadSHA, m.CandidateSHA))
		}
		if !si.Dirty && sf.Dirty && sf.StatusQueryError == "" {
			reasons = append(reasons, "source tree became dirty during collection")
		}
	}
	allPass := true
	for _, cmd := range m.Commands {
		if cmd.Status != StatusPASS {
			allPass = false
			break
		}
	}
	if !allPass {
		reasons = append(reasons, "not every gate command finished PASS (see per-command statuses)")
	}
	if m.Interrupted {
		reasons = append(reasons, "collection was interrupted (SIGINT/SIGTERM); INTERRUPTED or RUNNING is never PASS")
	}
	m.BlockedReasons, m.FinishedUTC = reasons, nowUTC()
	if len(reasons) == 0 && allPass && !m.Interrupted {
		m.ReleaseStatus = StatusPASS
	} else {
		m.ReleaseStatus = StatusBLOCKED
	}
}

// persist writes manifest.json atomically (temp file + fsync + rename).
func (c *Collector) persist() error {
	data, err := json.MarshalIndent(c.m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := filepath.Join(c.OutputDir, "manifest.json.tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(c.OutputDir, "manifest.json")); err != nil {
		return err
	}
	return c.writeSummary()
}

func (c *Collector) writeSummary() error {
	m, b := c.m, &strings.Builder{}
	fmt.Fprintf(b, "# Release evidence: LOCAL_TESTS_VALIDATED (input)\n\n")
	fmt.Fprintf(b, "- Generated (UTC): %s\n- Collector: `%s`\n- Candidate SHA: `%s`\n", m.FinishedUTC, strings.Join(m.CollectorArgv, " "), m.CandidateSHA)
	fmt.Fprintf(b, "- Repo root: `%s`\n- Output dir: `%s`\n- Allow-dirty: %v\n- Claimed exact candidate: %v\n", m.RepoRoot, m.OutputDir, m.AllowDirty, m.ClaimedExactCandidate)
	if si, sf := m.SourceInitial, m.SourceFinal; si != nil && sf != nil {
		fmt.Fprintf(b, "- Source initial: HEAD `%s`, dirty=%v (%s)\n- Source final: HEAD `%s`, dirty=%v (%s)\n", si.HeadSHA, si.Dirty, si.CapturedUTC, sf.HeadSHA, sf.Dirty, sf.CapturedUTC)
	}
	if t := m.Toolchain; t != nil {
		fmt.Fprintf(b, "- Go toolchain: `%s` -> %s (GOVERSION=%s GOOS=%s GOARCH=%s)\n", t.GoPath, firstLine([]byte(t.VersionOutput)), t.GOVersion, t.GOOS, t.GOARCH)
		if t.ExpectedMinor != "" {
			fmt.Fprintf(b, "- Go minor: expected %s, actual %s\n", t.ExpectedMinor, orDefault(t.ActualMinor, "unknown"))
		}
	}
	b.WriteString("\n## Commands and identity queries\n\n")
	b.WriteString("| # | Command | Status | Exit | Start UTC | End UTC | stdout (file, bytes, SHA256) | stderr (file, bytes, SHA256) |\n|---|---------|--------|------|-----------|---------|------------------------------|------------------------------|\n")
	for _, cmd := range append(append([]*CommandRecord{}, m.Commands...), m.Queries...) {
		exit := "n/a"
		if cmd.ExitCode != nil {
			exit = strconv.Itoa(*cmd.ExitCode)
		}
		fmt.Fprintf(b, "| %d | `%s` | %s | %s | %s | %s | `%s`, %d, `%s` | `%s`, %d, `%s` |\n", cmd.ID, strings.Join(cmd.Argv, " "), cmd.Status, exit, cmd.StartUTC, cmd.EndUTC, cmd.StdoutFile, cmd.StdoutBytes, cmd.StdoutSHA256, cmd.StderrFile, cmd.StderrBytes, cmd.StderrSHA256)
	}
	b.WriteString("\nRaw bytes are in `<NN>-<slug>.stdout` / `.stderr` next to this file; SHA256 values match manifest.json.\n")
	fmt.Fprintf(b, "\n## Release status: %s\n", m.ReleaseStatus)
	if len(m.BlockedReasons) == 0 {
		b.WriteString("\nNo blocked reasons recorded.\n")
	} else {
		b.WriteString("\nBlocked reasons:\n\n")
		for _, r := range m.BlockedReasons {
			fmt.Fprintf(b, "- %s\n", r)
		}
	}
	b.WriteString("\n## Scope\n\n")
	for _, d := range m.Disclaimers {
		fmt.Fprintf(b, "- %s\n", d)
	}
	tmp := filepath.Join(c.OutputDir, "summary.md.tmp")
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(c.OutputDir, "summary.md"))
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// minorOfGOVERSION turns "go1.27.1" into "1.27"; "" when unparseable.
var goVersionPattern = regexp.MustCompile(`^go([0-9]+\.[0-9]+)(?:\.[0-9]+)?$`)

func minorOfGOVERSION(v string) string {
	match := goVersionPattern.FindStringSubmatch(v)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

// normalizeMinor accepts "1.27", "go1.27", or "1.27.3" and yields "1.27".
func normalizeMinor(s string) string {
	rest := strings.TrimPrefix(strings.TrimSpace(s), "go")
	parts := strings.SplitN(rest, ".", 3)
	if len(parts) < 2 {
		return rest
	}
	return parts[0] + "." + parts[1]
}
