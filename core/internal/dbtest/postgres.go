// Пакет dbtest поднимает одноразовый Postgres 16.15 для тестов.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const postgresImage = "postgres:16.15"

var (
	imageOnce sync.Once
	imageErr  error
)

// Start запускает контейнер postgres:16.15, создаёт базу ai_mentor_test
// и возвращает DSN без пароля. Контейнер удаляется в t.Cleanup.
// Если Docker или этот образ недоступны, тест пропускается.
func Start(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable: executable not found")
	}
	infoCtx, infoCancel := context.WithTimeout(context.Background(), 20*time.Second)
	_, infoErrText, err := dockerOutput(infoCtx, "info", "--format", "{{.ServerVersion}}")
	infoCancel()
	if err != nil {
		t.Skipf("docker unavailable: %s", trimText(infoErrText))
	}
	if err := ensureImage(); err != nil {
		t.Skipf("image %s unavailable: %s", postgresImage, trimText(err.Error()))
	}

	name := containerName()
	vol := name + "-data"
	runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Second)
	stdout, stderr, err := dockerOutput(runCtx, "run", "-d", "--rm", "--pull", "never",
		"--name", name,
		"-v", vol+":/var/lib/postgresql/data",
		"-e", "POSTGRES_HOST_AUTH_METHOD=trust",
		"-e", "POSTGRES_DB=ai_mentor_test",
		"-p", "127.0.0.1::5432",
		postgresImage,
	)
	runCancel()
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, _, _ = dockerOutput(ctx, "volume", "rm", vol)
		cancel()
		t.Fatalf("docker run: %v: %s", err, trimText(stderr))
	}
	id := strings.TrimSpace(stdout)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, _, rmErr := dockerOutput(ctx, "rm", "-f", id); rmErr != nil {
			t.Errorf("docker rm: %v", rmErr)
		}
		if _, _, rmErr := dockerOutput(ctx, "volume", "rm", vol); rmErr != nil {
			t.Errorf("docker volume rm: %v", rmErr)
		}
	})

	port := publishedPort(t, id)
	dsn := (&url.URL{
		Scheme:   "postgres",
		User:     url.User("postgres"),
		Host:     net.JoinHostPort("127.0.0.1", port),
		Path:     "/ai_mentor_test",
		RawQuery: "sslmode=disable",
	}).String()
	waitReady(t, id, dsn)
	return dsn
}

func ensureImage() error {
	imageOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if _, _, err := dockerOutput(ctx, "image", "inspect", postgresImage); err == nil {
			return
		}
		if _, stderr, err := dockerOutput(ctx, "pull", postgresImage); err != nil {
			imageErr = fmt.Errorf("%w: %s", err, trimText(stderr))
			return
		}
		if _, stderr, err := dockerOutput(ctx, "image", "inspect", postgresImage); err != nil {
			imageErr = fmt.Errorf("image %s unavailable after pull: %w: %s", postgresImage, err, trimText(stderr))
		}
	})
	return imageErr
}

func containerName() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("ai-mentor-test-%d", time.Now().UnixNano())
	}
	return "ai-mentor-test-" + hex.EncodeToString(b[:])
}

func publishedPort(t *testing.T, id string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		stdout, stderr, err := dockerOutput(ctx, "port", id, "5432/tcp")
		cancel()
		if err == nil {
			port, parseErr := parsePort(stdout)
			if parseErr == nil {
				return port
			}
			lastErr = parseErr
		} else {
			lastErr = fmt.Errorf("%w: %s", err, trimText(stderr))
		}
		if time.Now().After(deadline) {
			t.Fatalf("docker port: %v", lastErr)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func parsePort(out string) (string, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		host, port, err := net.SplitHostPort(line)
		if err != nil || port == "" {
			continue
		}
		if host == "127.0.0.1" {
			return port, nil
		}
	}
	return "", fmt.Errorf("no 127.0.0.1 port in %q", strings.TrimSpace(out))
}

func waitReady(t *testing.T, id, dsn string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conn, err := pgx.Connect(ctx, dsn)
		if err == nil {
			err = conn.Ping(ctx)
			if closeErr := conn.Close(ctx); err == nil {
				err = closeErr
			}
		}
		cancel()
		if err == nil {
			return
		}
		last = err
		if !containerRunning(id) {
			t.Fatalf("postgres container exited: %v\n%s", last, containerLogs(id))
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("postgres not ready: %v\n%s", last, containerLogs(id))
}

func containerRunning(id string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, _, err := dockerOutput(ctx, "inspect", "-f", "{{.State.Running}}", id)
	return err == nil && strings.TrimSpace(stdout) == "true"
}

func containerLogs(id string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, stderr, err := dockerOutput(ctx, "logs", "--tail", "40", id)
	text := strings.TrimSpace(stdout + stderr)
	if err != nil && text == "" {
		text = err.Error()
	}
	return trimText(text)
}

func dockerOutput(ctx context.Context, args ...string) (string, string, error) {
	//nolint:gosec // G204: fixed docker binary and an argument list, not a shell.
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func trimText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return s[:400]
	}
	return s
}
