package main

import "testing"

func TestImportExtsIncludeHEIF(t *testing.T) {
	if _, ok := importExts[".heic"]; !ok {
		t.Fatal("missing .heic")
	}
	if _, ok := importExts[".heif"]; !ok {
		t.Fatal("missing .heif")
	}
}
