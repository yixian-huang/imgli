// Package servecache 为公开图流式出站提供本地磁盘缓存，避免每次 /t、/i 回源。
package servecache

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	DefaultMaxBytes     int64 = 512 << 20
	DefaultMaxFileBytes int64 = 20 << 20
)

// Cache 进程本地磁盘缓存。只缓存调用方判定可公开共享的字节。
type Cache struct {
	dir          string
	maxBytes     int64
	maxFileBytes int64
	mu           sync.Mutex
}

func New(dir string, maxBytes, maxFileBytes int64) (*Cache, error) {
	if dir == "" {
		return nil, os.ErrInvalid
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if maxFileBytes <= 0 {
		maxFileBytes = DefaultMaxFileBytes
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Cache{dir: dir, maxBytes: maxBytes, maxFileBytes: maxFileBytes}, nil
}

func (c *Cache) MaxFileBytes() int64 { return c.maxFileBytes }

func (c *Cache) filePath(key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, h[:2], h)
}

func (c *Cache) Get(key string) (*os.File, bool) {
	if c == nil {
		return nil, false
	}
	p := c.filePath(key)
	f, err := os.Open(p)
	if err != nil {
		return nil, false
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now)
	return f, true
}

func (c *Cache) Put(key string, data []byte) error {
	if c == nil || len(data) == 0 {
		return nil
	}
	if int64(len(data)) > c.maxFileBytes {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.evict(int64(len(data))); err != nil {
		return err
	}
	p := c.filePath(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

type cacheEnt struct {
	path string
	size int64
	mod  time.Time
}

func (c *Cache) evict(incoming int64) error {
	var ents []cacheEnt
	var used int64
	err := filepath.Walk(c.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".tmp" {
			return nil
		}
		ents = append(ents, cacheEnt{path: path, size: info.Size(), mod: info.ModTime()})
		used += info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	if used+incoming <= c.maxBytes {
		return nil
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].mod.Before(ents[j].mod) })
	for _, e := range ents {
		if used+incoming <= c.maxBytes {
			break
		}
		if os.Remove(e.path) == nil {
			used -= e.size
		}
	}
	return nil
}

// CopyFrom 把 reader 读入内存（受 maxFileBytes 限制）并写入缓存。超限返回 nil,false。
func (c *Cache) CopyFrom(key string, r io.Reader) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(r, c.maxFileBytes+1))
	if err != nil || int64(len(data)) > c.maxFileBytes {
		return nil, false
	}
	_ = c.Put(key, data)
	return data, true
}
