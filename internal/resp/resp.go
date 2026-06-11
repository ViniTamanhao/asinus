package resp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

// Parser reads RESP-encoded data from a buffered reader.
// Client commands are always Arrays of Bulk Strings.
type Parser struct {
	r *bufio.Reader
}

// NewParser creates a Parser that reads from r.
// The underlying reader is wrapper in a bufio.Reader with a 64KiB buffer.
func NewParser(r io.Reader) *Parser {
	return &Parser{r: bufio.NewReaderSize(r, 64<<10)}
}

// ReadCommand reads a RESP Array header (*<n>\r\n) followed by exactly n Bulk Strings ($<len>\r\n<data>\r\n) and returns their contents as a []string.
// A Null Bulk String ($-1) is returned as an empty string.
// An empty array (*0\r\n) returns a nil slice.
func (p *Parser) ReadCommand() ([]string, error) {
	line, err := p.r.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("resp: read array header: %w", err)
	}
	if len(line) < 4 || line[0] != '*' || line[len(line)-2] != '\r' {
		return nil, fmt.Errorf("resp: invalid array length %q", line)
	}

	conuntStr := string(line[1 : len(line)-2])
	count, err := strconv.Atoi(conuntStr)
	if err != nil {
		return nil, fmt.Errorf("resp: invalid array length %q", conuntStr)
	}
	if count < 0 {
		return nil, fmt.Errorf("resp: negative array length %d", count)
	}
	if count == 0 {
		return nil, nil
	}

	cmd := make([]string, 0, count)
	for i := range count {
		line, err := p.r.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("resp: read bulk header %d: %w", i, err)
		}
		if len(line) < 4 || line[0] != '$' || line[len(line)-2] != '\r' {
			return nil, fmt.Errorf("resp: malformed bulk header %d: %q", i, line)
		}
		lenStr := string(line[1 : len(line)-2])
		n, err := strconv.Atoi(lenStr)
		if err != nil {
			return nil, fmt.Errorf("resp: invalid bulk length %d: %q: %w", i, lenStr, err)
		}

		if n == -1 {
			// Null Bulk String handled gracefully
			cmd = append(cmd, "")
			continue
		}

		if n < 0 {
			return nil, fmt.Errorf("resp: negative bulk length %d at index %d", n, i)
		}

		buf := make([]byte, n)
		if _, err := io.ReadFull(p.r, buf); err != nil {
			return nil, fmt.Errorf("resp: read bulk body %d: %w", i, err)
		}

		var trailer [2]byte
		if _, err := io.ReadFull(p.r, trailer[:]); err != nil {
			return nil, fmt.Errorf("resp: read bulk trailer %d: %w", i, err)
		}

		if trailer[0] != '\r' || trailer[1] != '\n' {
			return nil, fmt.Errorf("resp: expected CRLF after bulk body %d, got %q", i, trailer[:])
		}

		cmd = append(cmd, string(buf))
	}

	return cmd, nil
}

// Writer serializes RESP values onto an io.Writer.
// The caller may optionally wrap the io.Writer in a bufio.Writer for buffered writes.
type Writer struct {
	w io.Writer
}

// NewWriter creates a WRiter that writes to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteSimpleString writes a RESP Simple String: +<s>\r\n.
func (w *Writer) WriteSimpleString(s string) error {
	_, err := fmt.Fprintf(w.w, "+%s\r\n", s)
	return err
}

// WriteError writes a RESP Error: -<err.Error()>\r\n.
func (w *Writer) WriteError(errVal error) error {
	_, err := fmt.Fprintf(w.w, "-%s\r\n", errVal.Error())
	return err
}

// WriteBulk writes a RESP Bulk String: $<len>\r\n<bytes>\r\n.
// If b is nil it writes the Null Bulk String: $-1\r\n.
func (w *Writer) WriteBulk(b []byte) error {
	if b == nil {
		_, err := io.WriteString(w.w, "$-1\r\n")
		return err
	}

	if _, err := fmt.Fprintf(w.w, "$%d\r\n", len(b)); err != nil {
		return err
	}
	if _, err := w.w.Write(b); err != nil {
		return err
	}
	_, err := io.WriteString(w.w, "\r\n")
	return err
}

// WriteInt writes a RESP Integer: :<n>\r\n.
func (w *Writer) WriteInt(n int) error {
	_, err := fmt.Fprintf(w.w, ":%d\r\n", n)
	return err
}
