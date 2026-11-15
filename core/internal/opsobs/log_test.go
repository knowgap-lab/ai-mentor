package opsobs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestOP_R15_AllowsStudentIDAndPseudonym(t *testing.T) {
	var buf bytes.Buffer
	New(&buf).Info("проверка", "student_id", "stu-demo", "pseudonym", "K7MQ2XP4")
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("журнал не JSON: %v %q", err, buf.String())
	}
	if line["msg"] != "проверка" || line["student_id"] != "stu-demo" || line["pseudonym"] != "K7MQ2XP4" {
		t.Fatalf("запись %+v", line)
	}
	for _, key := range []string{"display_name", "telegram_id", "birth_date", "name", "token"} {
		if _, ok := line[key]; ok {
			t.Fatalf("в записи есть %s", key)
		}
	}
}

func TestOP_R15_DropsPersonalFields(t *testing.T) {
	fields := []string{"display_name", "telegram_id", "birth_date", "name", "token"}
	for _, key := range fields {
		t.Run(key, func(t *testing.T) {
			var buf bytes.Buffer
			New(&buf).Info("проверка", "student_id", "stu-demo", key, "secret-value")
			if buf.Len() != 0 {
				t.Fatalf("запись с %s записана: %s", key, buf.String())
			}
			if strings.Contains(buf.String(), "secret-value") || strings.Contains(buf.String(), "stu-demo") {
				t.Fatalf("значение попало в журнал: %s", buf.String())
			}
		})
	}
}

func TestOP_R15_DropsBoundPersonalField(t *testing.T) {
	var buf bytes.Buffer
	New(&buf).With("telegram_id", "424242").Info("проверка", "student_id", "stu-demo")
	if buf.Len() != 0 {
		t.Fatalf("привязанное поле записано: %s", buf.String())
	}
}

func TestOP_R15_DropsGroupedPersonalField(t *testing.T) {
	var buf bytes.Buffer
	New(&buf).Info("проверка", slog.Group("user", "telegram_id", "424242"))
	if buf.Len() != 0 {
		t.Fatalf("группа записана: %s", buf.String())
	}
}
