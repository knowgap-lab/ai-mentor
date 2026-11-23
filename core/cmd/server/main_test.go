package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAU_R36_ProcessEntrypoint(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "server")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	run := exec.Command(bin)
	run.Dir = dir
	run.Env = append(os.Environ(), "ENV=prod", "AUTH_DEV_MODE=true")
	done := make(chan error, 1)
	go func() { done <- run.Run() }()
	select {
	case err := <-done:
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("wanted exit 1, got %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = run.Process.Kill()
		t.Fatal("process did not exit before listen (AU-R36)")
	}
}

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	newRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
