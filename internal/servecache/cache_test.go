package servecache

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestPutGetRoundTrip(t *testing.T) {
	c, err := New(t.TempDir(), 1<<20, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Put("abc", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	f, ok := c.Get("abc")
	if !ok {
		t.Fatal("want hit")
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestGetMiss(t *testing.T) {
	c, err := New(t.TempDir(), 1<<20, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("want miss")
	}
}

func TestPutSkipsOversize(t *testing.T) {
	c, err := New(t.TempDir(), 1<<20, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Put("big", []byte("12345")); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("big"); ok {
		t.Fatal("超限文件不应入缓存")
	}
}

func TestEvictOldest(t *testing.T) {
	c, err := New(t.TempDir(), 12, 12)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Put("a", bytes.Repeat([]byte("a"), 6)); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("b", bytes.Repeat([]byte("b"), 6)); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("c", bytes.Repeat([]byte("c"), 6)); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("最旧项应被淘汰")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("最新项应保留")
	}
}

func TestDisabledNil(t *testing.T) {
	if _, err := New("", 0, 0); err == nil {
		t.Fatal("空目录应失败")
	}
	_ = os.ErrNotExist
}
