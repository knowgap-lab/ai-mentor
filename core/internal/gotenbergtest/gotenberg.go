// Пакет gotenbergtest поднимает Gotenberg для тестов так же, как dbtest
// поднимает Postgres. Образ и его версия читаются из deploy/compose.prod.yml
// (сервис gotenberg). Своей копии версии пакет не хранит. Образ не скачивает:
// нет Docker или образа — тест пропускается.
package gotenbergtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	containerPort = "3000/tcp"
	listenPort    = "3000"
)

// Image возвращает образ сервиса gotenberg из deploy/compose.prod.yml.
// Пустое имя, тег latest и тег не из трёх чисел — ошибка.
func Image() (string, error) {
	dir, err := deployDir()
	if err != nil {
		return "", err
	}
	return imageFrom(filepath.Join(dir, "compose.prod.yml"))
}

// EnvName — единственное имя переменной адреса Gotenberg в deploy/secrets.md,
// deploy/README.md и обоих compose. Имени нет или их несколько — ошибка:
// тест имя не выбирает.
func EnvName() (string, error) {
	dir, err := deployDir()
	if err != nil {
		return "", err
	}
	var found []string
	seen := map[string]bool{}
	for _, name := range []string{"secrets.md", "README.md", "compose.prod.yml", "compose.staging.yml"} {
		raw, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: путь — файл репозитория, его выбирает помощник тестов, не запрос
		if err != nil {
			return "", err
		}
		for _, item := range envNames(string(raw)) {
			if !seen[item] {
				seen[item] = true
				found = append(found, item)
			}
		}
	}
	sort.Strings(found)
	switch len(found) {
	case 0:
		return "", errors.New("имя переменной адреса Gotenberg не записано в deploy/secrets.md и deploy/README.md")
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("несколько имён адреса Gotenberg: %s", strings.Join(found, ", "))
	}
}

// Start запускает контейнер Gotenberg на случайном порту 127.0.0.1 и
// возвращает базовый URL. --pull never: образ не скачивается.
// Нет Docker, сервиса в compose или локального образа — t.Skip.
func Start(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable: executable not found")
	}
	infoCtx, infoCancel := context.WithTimeout(context.Background(), 20*time.Second)
	_, infoErr, err := dockerOutput(infoCtx, "info", "--format", "{{.ServerVersion}}")
	infoCancel()
	if err != nil {
		t.Skipf("docker unavailable: %s", trimText(infoErr))
	}
	image, err := Image()
	if err != nil {
		t.Skip(err.Error())
	}
	inspectCtx, inspectCancel := context.WithTimeout(context.Background(), 20*time.Second)
	_, inspectErr, err := dockerOutput(inspectCtx, "image", "inspect", image)
	inspectCancel()
	if err != nil {
		t.Skipf("image %s unavailable: %s", image, trimText(inspectErr+err.Error()))
	}

	name := containerName()
	runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Second)
	stdout, stderr, err := dockerOutput(runCtx, "run", "-d", "--rm", "--pull", "never",
		"--name", name,
		"-p", "127.0.0.1::"+listenPort,
		image,
	)
	runCancel()
	if err != nil {
		t.Fatalf("docker run: %v: %s", err, trimText(stderr))
	}
	id := strings.TrimSpace(stdout)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, _, rmErr := dockerOutput(ctx, "rm", "-f", id); rmErr != nil {
			t.Errorf("docker rm: %v", rmErr)
		}
	})
	port := publishedPort(t, id)
	base := "http://" + net.JoinHostPort("127.0.0.1", port)
	waitReady(t, id, base)
	return base
}

// Stub — подставной сервер вместо Gotenberg. Каждый запрос получает новый PDF.
// Fail переводит следующие ответы в заданный статус.
type Stub struct {
	URL string

	mu     sync.Mutex
	bodies [][]byte
	code   int
	srv    *httptest.Server
}

// Serve поднимает подставной сервер. Тест закрывает его сам.
func Serve(t *testing.T) *Stub {
	t.Helper()
	stub := &Stub{code: http.StatusOK}
	stub.srv = httptest.NewServer(http.HandlerFunc(stub.serve))
	stub.URL = stub.srv.URL
	t.Cleanup(stub.srv.Close)
	return stub
}

// Fail задаёт статус следующих ответов.
func (s *Stub) Fail(code int) {
	s.mu.Lock()
	s.code = code
	s.mu.Unlock()
}

// Bodies возвращает тела запросов по порядку.
func (s *Stub) Bodies() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.bodies))
	for i := range s.bodies {
		out[i] = append([]byte(nil), s.bodies[i]...)
	}
	return out
}

func (s *Stub) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.bodies = append(s.bodies, body)
	n := len(s.bodies)
	code := s.code
	s.mu.Unlock()
	if code != http.StatusOK {
		w.WriteHeader(code)
		_, _ = w.Write([]byte("gotenberg error"))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(pdfBody(n))
}

func pdfBody(n int) []byte {
	return []byte(fmt.Sprintf("%%PDF-1.4\n%% s8-%d\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%%%EOF\n", n))
}

func imageFrom(path string) (string, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: путь — файл репозитория, его выбирает помощник тестов, не запрос
	if err != nil {
		return "", err
	}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	image := strings.TrimSpace(doc.Services["gotenberg"].Image)
	if image == "" {
		return "", errors.New("в deploy/compose.prod.yml нет сервиса gotenberg")
	}
	if !pinned(image) {
		return "", fmt.Errorf("образ Gotenberg не закреплён точной версией: %s", image)
	}
	return image, nil
}

func pinned(image string) bool {
	if strings.Contains(strings.ToLower(image), "latest") {
		return false
	}
	tag := image
	if i := strings.LastIndex(image, ":"); i >= 0 {
		tag = image[i+1:]
	}
	tag = strings.Split(tag, "@")[0]
	parts := strings.Split(tag, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func envNames(text string) []string {
	re := regexp.MustCompile(`\b([A-Z][A-Z0-9_]*GOTENBERG[A-Z0-9_]*)\b`)
	found := re.FindAllString(text, -1)
	if found == nil {
		return nil
	}
	return found
}

func deployDir() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(file))
	}
	for _, start := range starts {
		dir := start
		for {
			candidate := filepath.Join(dir, "deploy", "compose.prod.yml")
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return filepath.Join(dir, "deploy"), nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("каталог deploy не найден")
}

func containerName() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("ai-mentor-gotenberg-%d", time.Now().UnixNano())
	}
	return "ai-mentor-gotenberg-" + hex.EncodeToString(b[:])
}

func publishedPort(t *testing.T, id string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		stdout, stderr, err := dockerOutput(ctx, "port", id, containerPort)
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

func waitReady(t *testing.T, id, base string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				cancel()
				return
			}
			err = fmt.Errorf("health %d", res.StatusCode)
		}
		cancel()
		last = err
		if !containerRunning(id) {
			t.Fatalf("gotenberg container exited: %v\n%s", last, containerLogs(id))
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("gotenberg not ready: %v\n%s", last, containerLogs(id))
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
	//nolint:gosec // G204: фиксированный docker и список аргументов, не оболочка.
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
