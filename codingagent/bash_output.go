package codingagent

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/nankedr/pig/ai"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type bashOutput struct {
	decoder                          *encoding.Decoder
	decodedAny                       bool
	raw, pending                     []byte
	tail                             string
	totalBytes, lines, lastLineBytes int
	tailBoundary                     bool
	file                             *os.File
	path                             string
	err                              error
}

func (o *bashOutput) append(data []byte) {
	o.decode(data, false)
	if o.file == nil && (len(o.raw)+len(data) > DefaultMaxBytes || o.totalBytes > DefaultMaxBytes || o.totalLines() > DefaultMaxLines) {
		o.persist()
	}
	if o.file != nil {
		if _, err := o.file.Write(data); err != nil && o.err == nil {
			o.err = err
		}
	} else if o.err == nil {
		o.raw = append(o.raw, data...)
	}
}
func (o *bashOutput) persist() {
	if o.file != nil || o.err != nil {
		return
	}
	o.file, o.err = os.CreateTemp("", "pig-bash-*.log")
	if o.err != nil {
		return
	}
	o.path = o.file.Name()
	_, o.err = o.file.Write(o.raw)
	o.raw = nil
}
func (o *bashOutput) decode(data []byte, final bool) {
	data = append(o.pending, data...)
	if o.decoder == nil {
		o.decoder = unicode.UTF8.NewDecoder()
	}
	decoded := make([]byte, len(data)*3)
	written, consumed, err := o.decoder.Transform(decoded, data, final)
	if err != nil && err != transform.ErrShortSrc && o.err == nil {
		o.err = err
	}
	o.pending = append([]byte(nil), data[consumed:]...)
	text := string(decoded[:written])
	if !o.decodedAny && text != "" {
		o.decodedAny = true
		text = strings.TrimPrefix(text, "\ufeff")
	}

	o.totalBytes += len(text)
	o.lines += strings.Count(text, "\n")
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		o.lastLineBytes = len(text) - i - 1
	} else {
		o.lastLineBytes += len(text)
	}
	o.tail += text
	if len(o.tail) > DefaultMaxBytes*4 {
		start := len(o.tail) - DefaultMaxBytes*2
		for start < len(o.tail) && !utf8.RuneStart(o.tail[start]) {
			start++
		}
		o.tailBoundary = o.tail[start-1] == '\n'
		o.tail = strings.Clone(o.tail[start:])
	}
}
func (o *bashOutput) totalLines() int {
	if o.lastLineBytes > 0 {
		return o.lines + 1
	}
	return o.lines
}
func (o *bashOutput) snapshot() (TruncationResult, ai.JSONValue) {
	text := o.tail
	if !o.tailBoundary {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	trunc := TruncateTail(text)
	trunc.Truncated = o.totalLines() > DefaultMaxLines || o.totalBytes > DefaultMaxBytes
	if trunc.Truncated && trunc.TruncatedBy == "" {
		if o.totalBytes > DefaultMaxBytes {
			trunc.TruncatedBy = "bytes"
		} else {
			trunc.TruncatedBy = "lines"
		}
	}
	trunc.TotalLines, trunc.TotalBytes = o.totalLines(), o.totalBytes
	if trunc.Truncated {
		o.persist()
		return trunc, map[string]any{"truncation": readToolTruncationJSON(trunc), "fullOutputPath": o.path}
	}
	if o.path != "" {
		return trunc, map[string]any{"fullOutputPath": o.path}
	}
	return trunc, nil
}
func (o *bashOutput) finish() (string, ai.JSONValue, error) {
	o.decode(nil, true)
	trunc, details := o.snapshot()
	if o.file != nil {
		if err := o.file.Close(); err != nil && o.err == nil {
			o.err = err
		}
	}
	if o.err != nil {
		return "", nil, o.err
	}
	text := trunc.Content
	if !trunc.Truncated {
		details = nil
	}
	if trunc.Truncated {
		start, end := trunc.TotalLines-trunc.OutputLines+1, trunc.TotalLines
		switch {
		case trunc.LastLinePartial:
			text += fmt.Sprintf("\n\n[Showing last %s of line %d (line is %s). Full output: %s]", FormatSize(int64(trunc.OutputBytes)), end, FormatSize(int64(o.lastLineBytes)), o.path)
		case trunc.TruncatedBy == "lines":
			text += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Full output: %s]", start, end, end, o.path)
		default:
			text += fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Full output: %s]", start, end, end, FormatSize(DefaultMaxBytes), o.path)
		}
	}
	return text, details, nil
}
