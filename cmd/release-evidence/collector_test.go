package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testCandidate = "a1b2c3d4e5a1b2c3d4e5a1b2c3d4e5a1b2c3d4" // 40 hex chars

// scriptedRunner dispatches on the joined argv, falling back to exit 0.
type scriptedRunner struct {
	script map[string]ExecResult
	calls  []string
	hook   func(callNo int, key string, c *Collector)
	c      *Collector
}

func (f *scriptedRunner) Run(argv []string) ExecResult {
	key := strings.Join(argv, " ")
	f.calls = append(f.calls, key)
	if f.hook != nil {
		f.hook(len(f.calls), key, f.c)
	}
	if res, ok := f.script[key]; ok {
		return res
	}
	return ExecResult{ExitCode: 0, Start: time.Now().UTC(), End: time.Now().UTC()}
}

func baseScript() map[string]ExecResult {
	return map[string]ExecResult{
		"git rev-parse HEAD":                           {ExitCode: 0, Stdout: []byte(testCandidate + "\n")},
		"git status --porcelain --untracked-files=all": {ExitCode: 0},
		"go version":                                   {ExitCode: 0, Stdout: []byte("go version go1.27.1 darwin/arm64\n")},
		"go env GOVERSION GOOS GOARCH":                 {ExitCode: 0, Stdout: []byte("go1.27.1\ndarwin\narm64\n")},
	}
}

func newCollector(t *testing.T, r *scriptedRunner) (*Collector, string) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "evidence")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Collector{
		RepoRoot:      t.TempDir(),
		OutputDir:     out,
		Candidate:     testCandidate,
		GoPath:        "go",
		Runner:        r,
		Ctx:           context.Background(),
		StartedUTC:    nowUTC(),
		ExpectedMinor: "1.27",
	}
	r.c = c
	return c, out
}

func readManifest(t *testing.T, dir string) *Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest.json: %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest.json: %v", err)
	}
	return &m
}

func statuses(m *Manifest) []string {
	out := make([]string, len(m.Commands))
	for i, cmd := range m.Commands {
		out[i] = cmd.Status
	}
	return out
}

func wantStatuses(t *testing.T, m *Manifest, want []string) {
	t.Helper()
	if got := statuses(m); !equal(got, want) {
		t.Fatalf("command statuses = %v, want %v", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasReason(m *Manifest, substr string) bool {
	for _, r := range m.BlockedReasons {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}

func countCalls(r *scriptedRunner, key string) int {
	n := 0
	for _, k := range r.calls {
		if k == key {
			n++
		}
	}
	return n
}

func TestCollectAllPass(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	r.script["go test -count=1 ./..."] = ExecResult{ExitCode: 0, Stdout: []byte("ok  \tgithub.com/cottondesu/mandala\t0.5s\n")}
	c, out := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	wantStatuses(t, m, []string{"PASS", "PASS", "PASS", "PASS", "PASS", "PASS"})
	if m.ReleaseStatus != StatusPASS || !m.ClaimedExactCandidate || m.Interrupted {
		t.Fatalf("release_status=%s claimed=%v interrupted=%v; want PASS/true/false", m.ReleaseStatus, m.ClaimedExactCandidate, m.Interrupted)
	}
	disk := readManifest(t, out)
	if disk.ReleaseStatus != StatusPASS || len(disk.BlockedReasons) != 0 {
		t.Fatalf("disk manifest status=%s reasons=%v", disk.ReleaseStatus, disk.BlockedReasons)
	}
	if got := disk.Toolchain; got.GOVersion != "go1.27.1" || got.GOOS != "darwin" || got.GOARCH != "arm64" || got.ActualMinor != "1.27" {
		t.Fatalf("toolchain recorded wrong: %+v", got)
	}
	raw, err := os.ReadFile(filepath.Join(out, "01-go-test-count1.stdout"))
	if err != nil {
		t.Fatal(err)
	}
	want := "ok  \tgithub.com/cottondesu/mandala\t0.5s\n"
	if string(raw) != want {
		t.Fatalf("stdout capture = %q, want %q", raw, want)
	}
	if disk.Commands[0].StdoutBytes != len(want) || disk.Commands[0].StdoutSHA256 != sha256Hex([]byte(want)) {
		t.Fatalf("stdout sha/size mismatch: %+v", disk.Commands[0])
	}
	sum, err := os.ReadFile(filepath.Join(out, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, substr := range []string{"Release status: PASS", sha256Hex([]byte(want)), testCandidate, "LOCAL_TESTS_VALIDATED"} {
		if !strings.Contains(string(sum), substr) {
			t.Fatalf("summary.md missing %q", substr)
		}
	}
}

func TestCollectFailFastMarksLaterCommandsNotRun(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	r.script["go test -shuffle=on -count=1 ./..."] = ExecResult{ExitCode: 1, Stderr: []byte("--- FAIL: TestX\nFAIL\n")}
	c, out := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	wantStatuses(t, m, []string{"PASS", "FAIL", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN"})
	if m.ReleaseStatus != StatusBLOCKED || !hasReason(m, "not every gate command finished PASS") {
		t.Fatalf("release_status=%s reasons=%v", m.ReleaseStatus, m.BlockedReasons)
	}
	// The final source check is collected even after a command failure.
	if n := countCalls(r, "git rev-parse HEAD"); n != 2 {
		t.Fatalf("git rev-parse HEAD called %d times, want 2 (initial+final)", n)
	}
	disk := readManifest(t, out)
	if disk.SourceFinal == nil || disk.SourceFinal.HeadSHA != testCandidate {
		t.Fatalf("final source snapshot missing: %+v", disk.SourceFinal)
	}
}

func TestCollectDirtyTreeBlockedWithoutAllowDirty(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	r.script["git status --porcelain --untracked-files=all"] = ExecResult{
		ExitCode: 0, Stdout: []byte(" M internal/mandala/batch_input.go\n?? .serena/\n")}
	c, out := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	wantStatuses(t, m, []string{"NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN"})
	if m.ReleaseStatus != StatusBLOCKED || m.ClaimedExactCandidate || !hasReason(m, "dirty") {
		t.Fatalf("status=%s claimed=%v reasons=%v", m.ReleaseStatus, m.ClaimedExactCandidate, m.BlockedReasons)
	}
	if countCalls(r, "go test -count=1 ./...") != 0 {
		t.Fatal("gate commands must not run on a dirty tree without -allow-dirty")
	}
	if _, err := os.Stat(filepath.Join(out, "01-go-test-count1.stdout")); !os.IsNotExist(err) {
		t.Fatal("no capture files expected for NOT_RUN commands")
	}
}

func TestCollectDirtyAllowDirtyRunsButStaysBlocked(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	r.script["git status --porcelain --untracked-files=all"] = ExecResult{
		ExitCode: 0, Stdout: []byte(" M internal/mandala/batch_input.go\n")}
	c, _ := newCollector(t, r)
	c.AllowDirty = true
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	wantStatuses(t, m, []string{"PASS", "PASS", "PASS", "PASS", "PASS", "PASS"})
	if m.ReleaseStatus != StatusBLOCKED || m.ClaimedExactCandidate || !hasReason(m, "-allow-dirty") {
		t.Fatalf("status=%s claimed=%v reasons=%v", m.ReleaseStatus, m.ClaimedExactCandidate, m.BlockedReasons)
	}
}

func TestCollectMovedSHABlockedNotRun(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	r.script["git rev-parse HEAD"] = ExecResult{ExitCode: 0, Stdout: []byte(strings.Repeat("f", 40) + "\n")}
	c, _ := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	wantStatuses(t, m, []string{"NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN"})
	if m.ReleaseStatus != StatusBLOCKED || m.ClaimedExactCandidate || !hasReason(m, "is not the expected candidate") {
		t.Fatalf("status=%s claimed=%v reasons=%v", m.ReleaseStatus, m.ClaimedExactCandidate, m.BlockedReasons)
	}
}

func TestCollectWrongGoMinorBlockedNotRun(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	c, _ := newCollector(t, r)
	c.ExpectedMinor = "1.26" // actual toolchain reports go1.27.1
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	wantStatuses(t, m, []string{"NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN"})
	if m.ReleaseStatus != StatusBLOCKED || !hasReason(m, `-expected-go-minor "1.26"`) {
		t.Fatalf("status=%s reasons=%v", m.ReleaseStatus, m.BlockedReasons)
	}
	if m.Toolchain.MinorMatches == nil || *m.Toolchain.MinorMatches {
		t.Fatalf("minor_matches = %v, want false", m.Toolchain.MinorMatches)
	}
}

func TestCollectLaunchErrorDistinctFromExitCode(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	r.script["go vet ./..."] = ExecResult{LaunchError: `exec: "/no/such/go": stat /no/such/go: no such file or directory`}
	c, out := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	wantStatuses(t, m, []string{"PASS", "PASS", "PASS", "LAUNCH_ERROR", "NOT_RUN", "NOT_RUN"})
	disk := readManifest(t, out)
	vet := disk.Commands[3]
	if vet.ExitCode != nil || vet.LaunchError == "" || vet.StdoutFile != "" {
		t.Fatalf("launch failure must be distinct from an exit code: %+v", vet)
	}
	if m.ReleaseStatus != StatusBLOCKED {
		t.Fatalf("release_status=%s, want BLOCKED", m.ReleaseStatus)
	}
}

func TestRunningPersistedBeforeLaunchAndInterrupted(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	r.script["go vet ./..."] = ExecResult{ExitCode: -1, Interrupted: true, Stderr: []byte("signal: killed\n")}
	r.hook = func(callNo int, key string, c *Collector) {
		if key != "go vet ./..." {
			return
		}
		summary, err := os.ReadFile(filepath.Join(c.OutputDir, "summary.md"))
		if err != nil || !strings.Contains(string(summary), "Release status: RUNNING") {
			t.Fatalf("mid-run summary = %q, err %v", summary, err)
		}
		// Observed while the command is executing: a hard kill at this point
		// must leave the persisted manifest at RUNNING, never PASS.
		mid := readManifest(t, c.OutputDir)
		if got := statuses(mid); !equal(got, []string{"PASS", "PASS", "PASS", "RUNNING", "NOT_RUN", "NOT_RUN"}) {
			t.Fatalf("mid-run manifest statuses = %v, want cmd4 RUNNING", got)
		}
	}
	c, out := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	wantStatuses(t, m, []string{"PASS", "PASS", "PASS", "INTERRUPTED", "NOT_RUN", "NOT_RUN"})
	if !m.Interrupted || m.ReleaseStatus != StatusBLOCKED || !hasReason(m, "interrupted") {
		t.Fatalf("interrupted=%v status=%s reasons=%v", m.Interrupted, m.ReleaseStatus, m.BlockedReasons)
	}
	disk := readManifest(t, out)
	if disk.Commands[3].Status != StatusINTERRUPTED || disk.Commands[3].ExitCode == nil || *disk.Commands[3].ExitCode != -1 {
		t.Fatalf("interrupted record wrong: %+v", disk.Commands[3])
	}
}

func TestCollectHeadChangedDuringCollection(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	moved := strings.Repeat("e", 40)
	seen := false
	r.hook = func(callNo int, key string, c *Collector) {
		if key != "git rev-parse HEAD" {
			return
		}
		if seen { // second (final) call: swap in the moved HEAD before its lookup
			r.script["git rev-parse HEAD"] = ExecResult{ExitCode: 0, Stdout: []byte(moved + "\n")}
		}
		seen = true
	}
	c, _ := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	if m.ClaimedExactCandidate || m.ReleaseStatus != StatusBLOCKED || !hasReason(m, "HEAD changed during collection") || !hasReason(m, "final HEAD") {
		t.Fatalf("status=%s reasons=%v", m.ReleaseStatus, m.BlockedReasons)
	}
}

func TestGoPathSubstitution(t *testing.T) {
	r := &scriptedRunner{script: map[string]ExecResult{
		"git rev-parse HEAD":                           {ExitCode: 0, Stdout: []byte(testCandidate + "\n")},
		"git status --porcelain --untracked-files=all": {ExitCode: 0},
		"/toolchain/go version":                        {ExitCode: 0, Stdout: []byte("go version go1.26.4 linux/amd64\n")},
		"/toolchain/go env GOVERSION GOOS GOARCH":      {ExitCode: 0, Stdout: []byte("go1.26.4\nlinux\namd64\n")},
	}}
	c, _ := newCollector(t, r)
	c.GoPath = "/toolchain/go"
	c.ExpectedMinor = "1.26"
	if err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := c.Manifest()
	if m.ReleaseStatus != StatusPASS {
		t.Fatalf("release_status=%s reasons=%v", m.ReleaseStatus, m.BlockedReasons)
	}
	for i, cmd := range m.Commands {
		if cmd.Argv[0] != "/toolchain/go" && cmd.Argv[0] != "git" {
			t.Fatalf("command %d argv not substituted: %v", i, cmd.Argv)
		}
	}
	if m.Toolchain.GOOS != "linux" || m.Toolchain.GOARCH != "amd64" || *m.Toolchain.MinorMatches != true {
		t.Fatalf("toolchain wrong: %+v", m.Toolchain)
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestCommandSetMatchesReleaseGate(t *testing.T) {
	want := [][]string{
		{"go", "test", "-count=1", "./..."},
		{"go", "test", "-shuffle=on", "-count=1", "./..."},
		{"go", "test", "-race", "-shuffle=on", "-count=1", "./..."},
		{"go", "vet", "./..."},
		{"go", "build", "./..."},
		{"git", "diff", "--check"},
	}
	if len(gateCommands) != len(want) {
		t.Fatalf("%d gate commands, want %d", len(gateCommands), len(want))
	}
	for i, spec := range gateCommands {
		if fmt.Sprint(spec.argv) != fmt.Sprint(want[i]) {
			t.Fatalf("gate command %d = %v, want %v", i, spec.argv, want[i])
		}
	}
}

func TestMetadataEvidenceAndToolchainValidation(t *testing.T) {
	for _, tc := range []struct{ name, minor, env, version string }{
		{"missing minor", "", "go1.27.1\nlinux\namd64\n", "go version go1.27.1 linux/amd64\n"},
		{"empty env", "1.27", "", "go version go1.27.1 linux/amd64\n"},
		{"malformed env", "1.27", "bad\nlinux\namd64\n", "go version go1.27.1 linux/amd64\n"},
		{"disagree", "1.27", "go1.27.1\nlinux\namd64\n", "go version go1.26.10 linux/amd64\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &scriptedRunner{script: baseScript()}
			r.script["go env GOVERSION GOOS GOARCH"] = ExecResult{Stdout: []byte(tc.env)}
			r.script["go version"] = ExecResult{Stdout: []byte(tc.version)}
			c, _ := newCollector(t, r)
			c.ExpectedMinor = tc.minor
			if err := c.Collect(); err != nil {
				t.Fatal(err)
			}
			if c.m.ReleaseStatus != StatusBLOCKED {
				t.Fatalf("status %s", c.m.ReleaseStatus)
			}
		})
	}
	r := &scriptedRunner{script: baseScript()}
	stderr := []byte("first line\nsecond line\n")
	r.script["go version"] = ExecResult{ExitCode: 7, Stderr: stderr, Start: time.Now(), End: time.Now()}
	c, out := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatal(err)
	}
	if len(c.m.Queries) != 6 {
		t.Fatalf("queries = %d", len(c.m.Queries))
	}
	q := c.m.Queries[2]
	raw, err := os.ReadFile(filepath.Join(out, q.StderrFile))
	if err != nil || string(raw) != string(stderr) || q.StderrSHA256 != sha256Hex(stderr) || q.ExitCode == nil || *q.ExitCode != 7 || q.StartUTC == "" || q.EndUTC == "" {
		t.Fatalf("query=%+v raw=%q err=%v", q, raw, err)
	}
}

func TestFinalSourceFailureClearsClaim(t *testing.T) {
	for _, result := range []ExecResult{{ExitCode: 1, Stderr: []byte("failure\n")}, {Stdout: []byte(" M changed\n")}} {
		r := &scriptedRunner{script: baseScript()}
		n := 0
		r.hook = func(_ int, key string, _ *Collector) {
			if key == "git status --porcelain --untracked-files=all" {
				n++
				if n == 2 {
					r.script[key] = result
				}
			}
		}
		c, _ := newCollector(t, r)
		if err := c.Collect(); err != nil {
			t.Fatal(err)
		}
		if c.m.ClaimedExactCandidate || c.m.ReleaseStatus != StatusBLOCKED {
			t.Fatalf("claim=%v status=%s", c.m.ClaimedExactCandidate, c.m.ReleaseStatus)
		}
	}
}

func TestCollectWaitErrorFailsAndPreservesCaptures(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	stdout, stderr := []byte("partial stdout\n"), []byte("partial stderr\n")
	r.script["go test -count=1 ./..."] = ExecResult{ExitCode: 0, WaitError: "output pipes held open", Stdout: stdout, Stderr: stderr}
	c, out := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatal(err)
	}
	m := readManifest(t, out)
	wantStatuses(t, m, []string{"FAIL", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN"})
	rec := m.Commands[0]
	if m.ReleaseStatus == StatusPASS || rec.ExitCode == nil || *rec.ExitCode != 0 || rec.WaitError == "" || rec.LaunchError != "" {
		t.Fatalf("unexpected release or command: %s %+v", m.ReleaseStatus, rec)
	}
	for _, capture := range []struct {
		name, hash string
		want       []byte
	}{{rec.StdoutFile, rec.StdoutSHA256, stdout}, {rec.StderrFile, rec.StderrSHA256, stderr}} {
		raw, err := os.ReadFile(filepath.Join(out, capture.name))
		if err != nil || string(raw) != string(capture.want) || capture.hash != sha256Hex(capture.want) {
			t.Errorf("capture %q: raw=%q hash=%s error=%v", capture.name, raw, capture.hash, err)
		}
	}
}

func TestCollectWaitErrorBlocksQueries(t *testing.T) {
	for _, key := range []string{"git rev-parse HEAD", "git status --porcelain --untracked-files=all", "go version", "go env GOVERSION GOOS GOARCH"} {
		t.Run(key, func(t *testing.T) {
			r := &scriptedRunner{script: baseScript()}
			result := r.script[key]
			result.WaitError = "output pipes held open"
			r.script[key] = result
			c, _ := newCollector(t, r)
			if err := c.Collect(); err != nil {
				t.Fatal(err)
			}
			m := c.Manifest()
			if m.ReleaseStatus == StatusPASS || m.Commands[0].Status != StatusNOTRUN {
				t.Fatalf("query wait error did not block: status=%s commands=%v", m.ReleaseStatus, statuses(m))
			}
			found := false
			for _, q := range m.Queries {
				if strings.Join(q.Argv, " ") == key && q.Status == StatusFAIL && q.WaitError != "" && q.ExitCode != nil && *q.ExitCode == 0 {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing failed query record: %+v", m.Queries)
			}
		})
	}
}

func TestCollectInterruptedWaitError(t *testing.T) {
	r := &scriptedRunner{script: baseScript()}
	r.script["go test -count=1 ./..."] = ExecResult{ExitCode: 0, WaitError: "output pipes held open", Interrupted: true, Stdout: []byte("partial\n")}
	c, out := newCollector(t, r)
	if err := c.Collect(); err != nil {
		t.Fatal(err)
	}
	m := readManifest(t, out)
	wantStatuses(t, m, []string{"INTERRUPTED", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN", "NOT_RUN"})
	if !m.Interrupted || m.Commands[0].WaitError == "" || m.Commands[0].ExitCode == nil || *m.Commands[0].ExitCode != 0 {
		t.Fatalf("unexpected interruption: %+v", m.Commands[0])
	}
}
