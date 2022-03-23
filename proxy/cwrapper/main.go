package cwrapper

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
)

func NewTypeWriterReadCloser(r io.ReadCloser) io.ReadCloser {
	return &typeWriterReadCloser{ReadCloser: r, time: time.Now()}
}

type typeWriterReadCloser struct {
	io.ReadCloser

	time   time.Time
	buffer bytes.Buffer
}

// I don't know why we do this, but it seems to work.
func sanitize(s string) string {
	s = strings.Replace(s, "\r", "", -1)
	s = strings.Replace(s, "\n", "<br/>", -1)
	s = strings.Replace(s, "'", "\\'", -1)
	s = strings.Replace(s, "\b", "<backspace>", -1)
	return s
}

func (lr *typeWriterReadCloser) Read(p []byte) (n int, err error) {
	n, err = lr.ReadCloser.Read(p)

	now := time.Now()

	// TBH I have no idea what this does.
	lr.buffer.WriteString(fmt.Sprintf(".wait(%d)", int(now.Sub(lr.time).Seconds()*1000)))
	lr.buffer.WriteString(fmt.Sprintf(".put('%s')", sanitize(string(p[:n]))))
	lr.time = now

	return n, err
}

func (lr *typeWriterReadCloser) String() string {
	return lr.buffer.String()
}

func (lr *typeWriterReadCloser) Close() error {
	return lr.ReadCloser.Close()
}
