package mandala

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestValidateBatchInputBytes(t *testing.T) {
	if err := validateBatchInputBytes([]byte(`{"batch_version":1}`)); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if err := validateBatchInputBytes([]byte("\xEF\xBB\xBF{}")); err == nil || !strings.Contains(err.Error(), "byte order mark") {
		t.Fatalf("BOM accepted: %v", err)
	}
	if err := validateBatchInputBytes([]byte("{\"id\":\"\xff\"}")); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 accepted: %v", err)
	}
}

func TestReadBoundedBatchInput(t *testing.T) {
	data, err := readBoundedBatchInput(strings.NewReader(strings.Repeat("x", maxBatchInputBytes)))
	if err != nil || len(data) != maxBatchInputBytes {
		t.Fatalf("boundary size rejected: len=%d err=%v", len(data), err)
	}
	_, err = readBoundedBatchInput(strings.NewReader(strings.Repeat("x", maxBatchInputBytes+1)))
	if err == nil || !isProblemCode(err, "E_LIMIT") {
		t.Fatalf("oversized input accepted: %v", err)
	}
}

func TestParseBatchInputAccepts(t *testing.T) {
	document := `{"batch_version":1,"operations":[` +
		`{"op":"add","id":"security"},` +
		`{"op":"add","id":"security.csrf","optional":true},` +
		`{"op":"mark","id":"security.csrf","status":"done"}` +
		`]}`
	ops, err := parseBatchInput([]byte(document))
	if err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	want := []BatchOperation{
		{Op: "add", ID: "security"},
		{Op: "add", ID: "security.csrf", Optional: true},
		{Op: "mark", ID: "security.csrf", Status: Done},
	}
	if len(ops) != len(want) {
		t.Fatalf("ops=%v want=%v", ops, want)
	}
	for i := range want {
		if ops[i] != want[i] {
			t.Fatalf("op[%d]=%+v want %+v", i, ops[i], want[i])
		}
	}

	// Field order at the root and inside operations must not matter.
	reordered := `{"operations":[{"id":"x","op":"add"}],"batch_version":1}`
	if ops, err = parseBatchInput([]byte(reordered)); err != nil || len(ops) != 1 || ops[0].ID != "x" {
		t.Fatalf("reordered document: ops=%v err=%v", ops, err)
	}
}

func TestParseBatchInputRejects(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{name: "empty input", input: "", wantMsg: "empty batch input"},
		{name: "whitespace only", input: "   \n\t ", wantMsg: "empty batch input"},
		{name: "array root", input: "[]", wantMsg: `expected "{"`},
		{name: "null root", input: "null", wantMsg: `expected "{"`},
		{name: "number root", input: "1", wantMsg: `expected "{"`},
		{name: "string root", input: `"batch"`, wantMsg: `expected "{"`},
		{name: "missing batch_version", input: `{"operations":[{"op":"add","id":"x"}]}`, wantMsg: `missing JSON field "batch_version"`},
		{name: "missing operations", input: `{"batch_version":1}`, wantMsg: `missing JSON field "operations"`},
		{name: "empty object", input: "{}", wantMsg: `missing JSON field "batch_version"`},
		{name: "unknown root key", input: `{"batch_version":1,"operations":[],"notes":[]}`, wantMsg: `unknown JSON field "notes"`},
		{name: "duplicate batch_version", input: `{"batch_version":1,"batch_version":1,"operations":[]}`, wantMsg: `duplicate JSON field "batch_version"`},
		{name: "duplicate operations", input: `{"batch_version":1,"operations":[],"operations":[]}`, wantMsg: `duplicate JSON field "operations"`},
		{name: "escaped duplicate root key", input: `{"batch_version":1,"\u0062atch_version":1,"operations":[]}`, wantMsg: `duplicate JSON field "batch_version"`},
		{name: "version 1.0", input: `{"batch_version":1.0,"operations":[]}`, wantMsg: "unsupported batch_version 1.0"},
		{name: "version 1e0", input: `{"batch_version":1e0,"operations":[]}`, wantMsg: "unsupported batch_version 1e0"},
		{name: "version 2", input: `{"batch_version":2,"operations":[]}`, wantMsg: "unsupported batch_version 2"},
		{name: "version zero", input: `{"batch_version":0,"operations":[]}`, wantMsg: "unsupported batch_version 0"},
		{name: "version string", input: `{"batch_version":"1","operations":[]}`, wantMsg: "batch_version must be the integer 1"},
		{name: "version null", input: `{"batch_version":null,"operations":[]}`, wantMsg: "batch_version must be the integer 1"},
		{name: "version boolean", input: `{"batch_version":true,"operations":[]}`, wantMsg: "batch_version must be the integer 1"},
		{name: "operations object", input: `{"batch_version":1,"operations":{}}`, wantMsg: `expected "["`},
		{name: "operations string", input: `{"batch_version":1,"operations":"add"}`, wantMsg: `expected "["`},
		{name: "operations null", input: `{"batch_version":1,"operations":null}`, wantMsg: `expected "["`},
		{name: "operations number", input: `{"batch_version":1,"operations":1}`, wantMsg: `expected "["`},
		{name: "operation not object", input: `{"batch_version":1,"operations":["add"]}`, wantMsg: `expected "{"`},
		{name: "operation number", input: `{"batch_version":1,"operations":[1]}`, wantMsg: `expected "{"`},
		{name: "operation null", input: `{"batch_version":1,"operations":[null]}`, wantMsg: `expected "{"`},
		{name: "missing op", input: `{"batch_version":1,"operations":[{"id":"x"}]}`, wantMsg: `missing "op"`},
		{name: "missing id", input: `{"batch_version":1,"operations":[{"op":"add"}]}`, wantMsg: `missing "id"`},
		{name: "unknown op kind", input: `{"batch_version":1,"operations":[{"op":"delete","id":"x"}]}`, wantMsg: `unknown batch op "delete"`},
		{name: "op wrong type", input: `{"batch_version":1,"operations":[{"op":1,"id":"x"}]}`, wantMsg: "expected a JSON string"},
		{name: "op null", input: `{"batch_version":1,"operations":[{"op":null,"id":"x"}]}`, wantMsg: "null is not allowed"},
		{name: "id wrong type", input: `{"batch_version":1,"operations":[{"op":"add","id":true}]}`, wantMsg: "expected a JSON string"},
		{name: "id null", input: `{"batch_version":1,"operations":[{"op":"add","id":null}]}`, wantMsg: "null is not allowed"},
		{name: "add with status", input: `{"batch_version":1,"operations":[{"op":"add","id":"x","status":"done"}]}`, wantMsg: `"status" is not allowed in add`},
		{name: "optional wrong type string", input: `{"batch_version":1,"operations":[{"op":"add","id":"x","optional":"yes"}]}`, wantMsg: "expected a JSON boolean"},
		{name: "optional wrong type number", input: `{"batch_version":1,"operations":[{"op":"add","id":"x","optional":0}]}`, wantMsg: "expected a JSON boolean"},
		{name: "optional null", input: `{"batch_version":1,"operations":[{"op":"add","id":"x","optional":null}]}`, wantMsg: "null is not allowed"},
		{name: "mark missing status", input: `{"batch_version":1,"operations":[{"op":"mark","id":"x"}]}`, wantMsg: `missing "status"`},
		{name: "mark status expanded", input: `{"batch_version":1,"operations":[{"op":"mark","id":"x","status":"expanded"}]}`, wantMsg: "invalid status"},
		{name: "mark status wrong case", input: `{"batch_version":1,"operations":[{"op":"mark","id":"x","status":"DONE"}]}`, wantMsg: "invalid status"},
		{name: "mark status wrong type", input: `{"batch_version":1,"operations":[{"op":"mark","id":"x","status":1}]}`, wantMsg: "expected a JSON string"},
		{name: "mark with optional false", input: `{"batch_version":1,"operations":[{"op":"mark","id":"x","status":"done","optional":false}]}`, wantMsg: `"optional" is not allowed in mark`},
		{name: "duplicate op field", input: `{"batch_version":1,"operations":[{"op":"add","op":"add","id":"x"}]}`, wantMsg: `duplicate JSON field "op"`},
		{name: "escaped duplicate id field", input: `{"batch_version":1,"operations":[{"op":"add","id":"x","\u0069d":"y"}]}`, wantMsg: `duplicate JSON field "id"`},
		{name: "duplicate status field", input: `{"batch_version":1,"operations":[{"op":"mark","id":"x","status":"done","status":"done"}]}`, wantMsg: `duplicate JSON field "status"`},
		{name: "escaped duplicate status field", input: `{"batch_version":1,"operations":[{"op":"mark","id":"x","status":"done","\u0073tatus":"open"}]}`, wantMsg: `duplicate JSON field "status"`},
		{name: "duplicate optional field", input: `{"batch_version":1,"operations":[{"op":"add","id":"x","optional":true,"optional":false}]}`, wantMsg: `duplicate JSON field "optional"`},
		{name: "escaped duplicate optional field", input: `{"batch_version":1,"operations":[{"op":"add","id":"x","optional":true,"\u006fptional":false}]}`, wantMsg: `duplicate JSON field "optional"`},
		{name: "unknown operation field", input: `{"batch_version":1,"operations":[{"op":"add","id":"x","note":"hi"}]}`, wantMsg: `unknown JSON field "note"`},
		{name: "case variant root field", input: `{"Batch_Version":1,"operations":[]}`, wantMsg: `unknown JSON field "Batch_Version"`},
		{name: "case variant op field", input: `{"batch_version":1,"operations":[{"Op":"add","id":"x"}]}`, wantMsg: `unknown JSON field "Op"`},
		{name: "case variant id field", input: `{"batch_version":1,"operations":[{"op":"add","ID":"x"}]}`, wantMsg: `unknown JSON field "ID"`},
		{name: "trailing object", input: `{"batch_version":1,"operations":[{"op":"add","id":"x"}]} {}`, wantMsg: "trailing JSON"},
		{name: "trailing number", input: `{"batch_version":1,"operations":[{"op":"add","id":"x"}]} 5`, wantMsg: "trailing JSON"},
		{name: "trailing comma", input: `{"batch_version":1,"operations":[{"op":"add","id":"x"}]},`, wantMsg: "invalid JSON"},
		{name: "malformed truncated", input: `{"batch_version":1,"operations":[{"op":"add"`, wantMsg: "invalid JSON"},
		{name: "malformed garbage", input: `not json at all`, wantMsg: "invalid JSON"},
		{name: "null operation field value", input: `{"batch_version":1,"operations":[{"op":"mark","id":"x","status":null}]}`, wantMsg: "null is not allowed"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var err error
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Fatalf("parser panicked: %v", recovered)
					}
				}()
				_, err = parseBatchInput([]byte(testCase.input))
			}()
			if err == nil {
				t.Fatalf("accepted invalid input %q", testCase.input)
			}
			if !isProblemCode(err, "E_BATCH_INVALID") {
				t.Fatalf("error code: %v", err)
			}
			if !strings.Contains(err.Error(), testCase.wantMsg) {
				t.Fatalf("error=%q want substring %q", err, testCase.wantMsg)
			}
		})
	}
}

func TestParseBatchVersionNumberBoundaries(t *testing.T) {
	for _, version := range []string{"01", "-01", "00", "1.", "1e", "0x1", "0", "2", "1.0", "1e0"} {
		for _, versionFirst := range []bool{true, false} {
			name := version + "/operations-first"
			input := `{"operations":[{"op":"add","id":"x"}],"batch_version":` + version + `}`
			if versionFirst {
				name = version + "/version-first"
				input = `{"batch_version":` + version + `,"operations":[{"op":"add","id":"x"}]}`
			}
			t.Run(name, func(t *testing.T) {
				_, err := parseBatchInput([]byte(input))
				want := "invalid JSON:"
				switch version {
				case "0", "2", "1.0", "1e0":
					want = "unsupported batch_version " + version
				}
				if !isProblemCode(err, "E_BATCH_INVALID") || err == nil || !strings.HasPrefix(err.Error(), want) {
					t.Fatalf("error = %v, want E_BATCH_INVALID with prefix %q", err, want)
				}
			})
		}
	}
}

func TestParseBatchVersionDiagnosticPrecedence(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`{"batch_version":2,"operations":`, "unsupported batch_version 2"},
		{`{"unknown":0,"batch_version":01}`, `unknown JSON field "unknown"`},
	} {
		_, err := parseBatchInput([]byte(tc.input))
		if err == nil || err.Error() != tc.want {
			t.Errorf("parseBatchInput(%q) = %v, want %q", tc.input, err, tc.want)
		}
	}
}

// TestParseBatchInputOperationCount proves the operation-count limits are
// enforced with E_LIMIT after the document itself parses: at least 1 and at
// most 128 operations.
// TestParseBatchInputTruncatedDiagnostics pins the exact message for every
// truncation shape. EOF-family decoder errors must normalize to one wording
// on every supported toolchain: Go 1.26's json v1 decoder reports truncation
// as io.EOF and would otherwise leak "invalid JSON: EOF", and a removed or
// reintroduced empty-input special case would misreport truncation as
// "empty batch input" (audit round 3, MINOR-1).
func TestParseBatchInputTruncatedDiagnostics(t *testing.T) {
	const want = "invalid JSON: unexpected end of JSON input"
	inputs := []string{
		`{`,
		`{"batch_ver`,
		`{"batch_version":1,`,
		`{"batch_version":1,"operations":`,
		`{"batch_version":1,"operations":[]`,
		`{"batch_version":1,"operations":[{"op":"add"`,
		`{"batch_version":1,"operations":[{"op":"add","id":"a"`,
		`{"batch_version":1,"operations":[{"op":"add","id":"a"}`,
		`{"batch_version":1,"operations":[{"op":"add","id":"a"},`,
		`{"batch_version":1,"operations":[{"op":"add","id":"aa`,
		`{"batch_version":1,"operations":[{"op":"add","id":"a","optional":tru`,
		`{"batch_version":1,"operations":[{"op":"mark","id":"a","status":`,
		`{"batch_version":1,"operations":[{"op":"mark","id":"a","status":"open`,
	}
	for _, input := range inputs {
		_, err := parseBatchInput([]byte(input))
		if err == nil {
			t.Errorf("parseBatchInput(%q) succeeded, want rejection", input)
			continue
		}
		if !isProblemCode(err, "E_BATCH_INVALID") {
			t.Errorf("parseBatchInput(%q): want E_BATCH_INVALID, got %v", input, err)
			continue
		}
		if got := err.Error(); got != want {
			t.Errorf("parseBatchInput(%q) message = %q, want %q", input, got, want)
		}
	}
}

func TestParseBatchInputOperationCount(t *testing.T) {
	_, err := parseBatchInput([]byte(`{"batch_version":1,"operations":[]}`))
	if err == nil || !isProblemCode(err, "E_LIMIT") || !strings.Contains(err.Error(), "at least 1 operation") {
		t.Fatalf("zero operations: %v", err)
	}
	builder := strings.Builder{}
	builder.WriteString(`{"batch_version":1,"operations":[`)
	for i := range 129 {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(`{"op":"add","id":"c"}`)
	}
	builder.WriteString(`]}`)
	_, err = parseBatchInput([]byte(builder.String()))
	if err == nil || !isProblemCode(err, "E_LIMIT") || !strings.Contains(err.Error(), "exceeds 128 operations") {
		t.Fatalf("129 operations: %v", err)
	}
}

// TestBatchDiagnosticTokenLimits pins the diagnostic-length contract for
// untrusted JSON tokens: op kinds, status values, unknown field names, and
// batch_version literals are capped at 64 bytes on a UTF-8 rune boundary
// before formatting, so a hostile ~64 KiB input cannot produce a ~64 KiB
// stderr line. Exact-message assertions also prove short values keep their
// pre-existing wording.
func TestBatchDiagnosticTokenLimits(t *testing.T) {
	ascii64 := strings.Repeat("x", 64)
	multi21 := strings.Repeat("\u3042", 21) // rune-boundary cap of 70 'あ' runes = 63 bytes
	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{
			name:    "huge op kind",
			input:   `{"batch_version":1,"operations":[{"op":"` + strings.Repeat("x", 60000) + `","id":"a"}]}`,
			wantMsg: `unknown batch op "` + ascii64 + `"`,
		},
		{
			name:    "huge status",
			input:   `{"batch_version":1,"operations":[{"op":"mark","id":"a","status":"` + strings.Repeat("x", 60000) + `"}]}`,
			wantMsg: `invalid status "` + ascii64 + `"; status must be open, done, or na`,
		},
		{
			name:    "huge unknown root field",
			input:   `{"` + strings.Repeat("x", 60000) + `":1,"batch_version":1,"operations":[]}`,
			wantMsg: `unknown JSON field "` + ascii64 + `"`,
		},
		{
			name:    "huge unknown operation field",
			input:   `{"batch_version":1,"operations":[{"op":"add","id":"a","` + strings.Repeat("x", 60000) + `":1}]}`,
			wantMsg: `unknown JSON field "` + ascii64 + `"`,
		},
		{
			name:    "huge batch_version literal",
			input:   `{"batch_version":` + strings.Repeat("9", 60000) + `,"operations":[]}`,
			wantMsg: `unsupported batch_version ` + strings.Repeat("9", 64),
		},
		{
			name:    "multibyte op kind caps on rune boundary",
			input:   `{"batch_version":1,"operations":[{"op":"` + strings.Repeat("\u3042", 70) + `","id":"a"}]}`,
			wantMsg: `unknown batch op "` + multi21 + `"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseBatchInput([]byte(tt.input))
			if err == nil {
				t.Fatal("expected rejection, got success")
			}
			if !isProblemCode(err, "E_BATCH_INVALID") {
				t.Fatalf("error code: %v", err)
			}
			if got := err.Error(); got != tt.wantMsg {
				t.Fatalf("diagnostic mismatch:\n got (%d bytes) %q\nwant (%d bytes) %q", len(got), got, len(tt.wantMsg), tt.wantMsg)
			}
		})
	}
}

func isProblemCode(err error, code string) bool {
	var problem *Problem
	if !errors.As(err, &problem) {
		return false
	}
	return problem.Code == code
}

// countingReader records how much of the source was actually consumed.
type countingReader struct {
	data string
	read int
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.read >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.read:])
	r.read += n
	return n, nil
}

// TestReadBoundedBatchInputNeverOverreads proves the bounded reader stops
// at the limit: over-limit input consumes exactly max+1 bytes, never the
// whole source, so the limit holds for stream sources (stdin) too.
func TestReadBoundedBatchInputNeverOverreads(t *testing.T) {
	over := &countingReader{data: strings.Repeat("a", maxBatchInputBytes+5)}
	if _, err := readBoundedBatchInput(over); !isProblemCode(err, "E_LIMIT") {
		t.Fatalf("over-limit input: %v", err)
	}
	if over.read != maxBatchInputBytes+1 {
		t.Fatalf("consumed %d bytes; bounded reader must read exactly %d", over.read, maxBatchInputBytes+1)
	}
	exact := &countingReader{data: strings.Repeat("a", maxBatchInputBytes)}
	data, err := readBoundedBatchInput(exact)
	if err != nil {
		t.Fatalf("exact-limit input rejected: %v", err)
	}
	if len(data) != maxBatchInputBytes {
		t.Fatalf("exact-limit input returned %d bytes", len(data))
	}
	if exact.read > maxBatchInputBytes {
		t.Fatalf("consumed %d bytes on exact-limit input; must not exceed %d", exact.read, maxBatchInputBytes)
	}
}
