package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"down"}, "postgres://postgres@127.0.0.1:1/nope", io.Discard, &stderr); code == 0 {
		t.Fatal("exit = 0")
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestUpRequiresDatabaseURL(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(nil, "", io.Discard, &stderr); code == 0 {
		t.Fatal("exit = 0")
	}
	if !strings.Contains(stderr.String(), "DATABASE_URL") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestFindMigrations(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := findMigrations(wd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "00001_schemas.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err := findMigrations(string(filepath.Separator)); err == nil {
		t.Fatal("expected error")
	}
}
