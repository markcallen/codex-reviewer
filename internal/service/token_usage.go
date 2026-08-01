package service

import (
	"bytes"
	"sync"

	"github.com/markcallen/codex-reviewer/internal/usage"
)

type tokenUsageCollector struct {
	mu     sync.Mutex
	buf    []byte
	usage  usage.ActualTokenUsage
	hasAny bool
}

func (c *tokenUsageCollector) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	for {
		idx := bytes.IndexByte(c.buf, '\n')
		if idx < 0 {
			if len(c.buf) > 64*1024 {
				c.buf = c.buf[len(c.buf)-64*1024:]
			}
			return len(p), nil
		}
		line := append([]byte(nil), c.buf[:idx]...)
		c.buf = c.buf[idx+1:]
		c.observeLineLocked(line)
	}
}

func (c *tokenUsageCollector) Usage() (usage.ActualTokenUsage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(bytes.TrimSpace(c.buf)) > 0 {
		c.observeLineLocked(c.buf)
		c.buf = nil
	}
	return c.usage, c.hasAny
}

func (c *tokenUsageCollector) observeLineLocked(line []byte) {
	actual, ok := usage.ParseActualUsageJSONLine(bytes.TrimSpace(line))
	if !ok {
		return
	}
	c.usage = actual
	c.hasAny = true
}
