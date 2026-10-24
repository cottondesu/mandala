package mandala

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const (
	// batchInputVersion is the only supported batch input contract version.
	batchInputVersion = 1
	// maxBatchInputBytes bounds the batch input document.
	maxBatchInputBytes = 65536
	// maxBatchOperations bounds the number of operations in one batch.
	maxBatchOperations = 128
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// readBoundedBatchInput reads at most maxBatchInputBytes+1 bytes and rejects
// anything longer, so oversized streams are never read unbounded.
func readBoundedBatchInput(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBatchInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBatchInputBytes {
		return nil, problem("E_LIMIT", "batch input exceeds %d bytes", maxBatchInputBytes)
	}
	return data, nil
}

// validateBatchInputBytes rejects byte-order marks and invalid UTF-8 before
// JSON parsing.
func validateBatchInputBytes(data []byte) error {
	if bytes.HasPrefix(data, utf8BOM) {
		return problem("E_BATCH_INVALID", "batch input must not begin with a UTF-8 byte order mark")
	}
	if !utf8.Valid(data) {
		return problem("E_BATCH_INVALID", "batch input is not valid UTF-8")
	}
	return nil
}

// parseBatchInput strictly parses the batch input document. The Go flag-free
// token walk enforces: exact root keys (batch_version, operations), each key
// exactly once (duplicate detection compares decoded key values, so escaped
// spellings cannot smuggle a duplicate), the integer literal 1 for
// batch_version (rejecting 1.0, 1e0, "1", null), no null values anywhere,
// per-operation key sets and types, and no trailing JSON values. Keys are
// matched case-sensitively.
func parseBatchInput(data []byte) ([]BatchOperation, error) {
	// Emptiness is decided on the raw bytes: a whitespace-only document must
	// report "empty batch input" on every toolchain, and any io.EOF surfaced
	// by the decoder below always means the document is truncated.
	if len(bytes.Trim(data, " \t\r\n")) == 0 {
		return nil, problem("E_BATCH_INVALID", "empty batch input")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	ops, err := parseBatchDocument(decoder)
	if err != nil {
		return nil, err
	}
	if err := expectBatchInputEOF(decoder); err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return nil, problem("E_LIMIT", "batch requires at least 1 operation")
	}
	if len(ops) > maxBatchOperations {
		return nil, problem("E_LIMIT", "batch exceeds %d operations", maxBatchOperations)
	}
	return ops, nil
}

func parseBatchDocument(decoder *json.Decoder) ([]BatchOperation, error) {
	if err := expectBatchDelim(decoder, '{'); err != nil {
		return nil, err
	}
	var ops []BatchOperation
	seenVersion, seenOperations := false, false
	for decoder.More() {
		key, err := nextBatchKey(decoder)
		if err != nil {
			return nil, err
		}
		switch key {
		case "batch_version":
			if seenVersion {
				return nil, problem("E_BATCH_INVALID", "duplicate JSON field %q", key)
			}
			seenVersion = true
			if err := parseBatchVersion(decoder); err != nil {
				return nil, err
			}
		case "operations":
			if seenOperations {
				return nil, problem("E_BATCH_INVALID", "duplicate JSON field %q", key)
			}
			seenOperations = true
			ops, err = parseBatchOperations(decoder)
			if err != nil {
				return nil, err
			}
		default:
			return nil, problem("E_BATCH_INVALID", "unknown JSON field %q", truncateBatchToken(key))
		}
	}
	if err := expectBatchDelim(decoder, '}'); err != nil {
		return nil, err
	}
	if !seenVersion {
		return nil, problem("E_BATCH_INVALID", "missing JSON field \"batch_version\"")
	}
	if !seenOperations {
		return nil, problem("E_BATCH_INVALID", "missing JSON field \"operations\"")
	}
	return ops, nil
}

func parseBatchVersion(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return batchParseError(err)
	}
	number, ok := token.(json.Number)
	if !ok {
		return problem("E_BATCH_INVALID", "batch_version must be the integer %d", batchInputVersion)
	}
	if number.String() != "1" {
		return problem("E_BATCH_INVALID", "unsupported batch_version %s", truncateBatchToken(number.String()))
	}
	return nil
}

func parseBatchOperations(decoder *json.Decoder) ([]BatchOperation, error) {
	if err := expectBatchDelim(decoder, '['); err != nil {
		return nil, err
	}
	ops := make([]BatchOperation, 0)
	for decoder.More() {
		op, err := parseBatchOperation(decoder)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	if err := expectBatchDelim(decoder, ']'); err != nil {
		return nil, err
	}
	return ops, nil
}

// parseBatchOperation parses one operation object. Fields may appear in any
// order, so keys are collected first and validated against the operation's
// key contract once "op" is known.
func parseBatchOperation(decoder *json.Decoder) (BatchOperation, error) {
	if err := expectBatchDelim(decoder, '{'); err != nil {
		return BatchOperation{}, err
	}
	var op, id, status *string
	var optional *bool
	for decoder.More() {
		key, err := nextBatchKey(decoder)
		if err != nil {
			return BatchOperation{}, err
		}
		switch key {
		case "op":
			if op != nil {
				return BatchOperation{}, problem("E_BATCH_INVALID", "duplicate JSON field %q", key)
			}
			value, err := expectBatchString(decoder)
			if err != nil {
				return BatchOperation{}, err
			}
			op = &value
		case "id":
			if id != nil {
				return BatchOperation{}, problem("E_BATCH_INVALID", "duplicate JSON field %q", key)
			}
			value, err := expectBatchString(decoder)
			if err != nil {
				return BatchOperation{}, err
			}
			id = &value
		case "status":
			if status != nil {
				return BatchOperation{}, problem("E_BATCH_INVALID", "duplicate JSON field %q", key)
			}
			value, err := expectBatchString(decoder)
			if err != nil {
				return BatchOperation{}, err
			}
			status = &value
		case "optional":
			if optional != nil {
				return BatchOperation{}, problem("E_BATCH_INVALID", "duplicate JSON field %q", key)
			}
			value, err := expectBatchBool(decoder)
			if err != nil {
				return BatchOperation{}, err
			}
			optional = &value
		default:
			return BatchOperation{}, problem("E_BATCH_INVALID", "unknown JSON field %q", truncateBatchToken(key))
		}
	}
	if err := expectBatchDelim(decoder, '}'); err != nil {
		return BatchOperation{}, err
	}
	if op == nil {
		return BatchOperation{}, problem("E_BATCH_INVALID", "operation is missing \"op\"")
	}
	if id == nil {
		return BatchOperation{}, problem("E_BATCH_INVALID", "operation is missing \"id\"")
	}
	switch *op {
	case "add":
		if status != nil {
			return BatchOperation{}, problem("E_BATCH_INVALID", "field \"status\" is not allowed in add operations")
		}
		return BatchOperation{Op: "add", ID: *id, Optional: optional != nil && *optional}, nil
	case "mark":
		if optional != nil {
			return BatchOperation{}, problem("E_BATCH_INVALID", "field \"optional\" is not allowed in mark operations")
		}
		if status == nil {
			return BatchOperation{}, problem("E_BATCH_INVALID", "mark operation is missing \"status\"")
		}
		switch *status {
		case "open", "done", "na":
		default:
			return BatchOperation{}, problem("E_BATCH_INVALID", "invalid status %q; status must be open, done, or na", truncateBatchToken(*status))
		}
		return BatchOperation{Op: "mark", ID: *id, Status: Status(*status)}, nil
	default:
		return BatchOperation{}, problem("E_BATCH_INVALID", "unknown batch op %q", truncateBatchToken(*op))
	}
}

func expectBatchDelim(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return batchParseError(err)
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != want {
		return problem("E_BATCH_INVALID", "invalid JSON: expected %q", want)
	}
	return nil
}

func nextBatchKey(decoder *json.Decoder) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", batchParseError(err)
	}
	key, ok := token.(string)
	if !ok {
		return "", problem("E_BATCH_INVALID", "invalid JSON: expected a field name")
	}
	return key, nil
}

func expectBatchString(decoder *json.Decoder) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", batchParseError(err)
	}
	switch value := token.(type) {
	case string:
		return value, nil
	case nil:
		return "", problem("E_BATCH_INVALID", "JSON null is not allowed in batch input")
	default:
		return "", problem("E_BATCH_INVALID", "expected a JSON string")
	}
}

func expectBatchBool(decoder *json.Decoder) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, batchParseError(err)
	}
	switch value := token.(type) {
	case bool:
		return value, nil
	case nil:
		return false, problem("E_BATCH_INVALID", "JSON null is not allowed in batch input")
	default:
		return false, problem("E_BATCH_INVALID", "expected a JSON boolean")
	}
}

func expectBatchInputEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return batchParseError(err)
	}
	return problem("E_BATCH_INVALID", "batch input contains trailing JSON")
}

// batchParseError normalizes decoder failures. Go 1.26's JSON decoder reports
// a truncated document as io.EOF (or io.ErrUnexpectedEOF) at any depth, while
// newer toolchains spell it "unexpected end of JSON input"; both are mapped
// to the same wording so truncation diagnostics do not depend on the
// toolchain. Other syntax-error wordings may still differ between the two
// JSON implementations; exit code, error code, stdout, and state handling
// stay identical either way.
func batchParseError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		err = errors.New("unexpected end of JSON input")
	}
	return problem("E_BATCH_INVALID", "invalid JSON: %v", err)
}
