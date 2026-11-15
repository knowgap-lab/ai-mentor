import { Button, Stack, Text, TextInput } from "@mantine/core";
import { type FormEvent, useLayoutEffect, useState } from "react";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";
import { List, ListButton, Screen } from "../ui";
import { minskTime } from "./minsk-time";

type Verdict = "correct" | "partial" | "wrong";

type StudentSummary = {
  id: string;
  display_name: string;
  next_lesson_at?: string;
  homework_waiting_review: number;
  homework_submitted: number;
  homework_overdue: number;
  open_disputes: number;
  not_delivered: boolean;
  warnings: string[];
};

type AttemptRow = {
  attempt_id: string;
  raw_input: string;
  verdict?: Verdict;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function readCount(value: unknown): number {
  return typeof value === "number" ? value : 0;
}

function readWarnings(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const texts: string[] = [];
  for (const item of value) {
    if (isRecord(item) && typeof item.text === "string" && item.text !== "") {
      texts.push(item.text);
    }
  }
  return texts;
}

function readStudent(value: unknown): StudentSummary | null {
  if (!isRecord(value)) {
    return null;
  }
  if (typeof value.id !== "string" || typeof value.display_name !== "string") {
    return null;
  }
  const nextLesson = value.next_lesson_at;
  return {
    id: value.id,
    display_name: value.display_name,
    next_lesson_at: typeof nextLesson === "string" ? nextLesson : undefined,
    homework_waiting_review: readCount(value.homework_waiting_review),
    homework_submitted: readCount(value.homework_submitted),
    homework_overdue: readCount(value.homework_overdue),
    open_disputes: readCount(value.open_disputes),
    not_delivered: value.not_delivered === true,
    warnings: readWarnings(value.warnings),
  };
}

function rowSubtitle(student: StudentSummary): string {
  const time = student.next_lesson_at ? minskTime(student.next_lesson_at) : "";
  return [
    time ? `следующий урок ${time}` : "",
    `ДЗ ждёт просмотра ${student.homework_waiting_review}`,
    `сдано ${student.homework_submitted}`,
    `просрочено ${student.homework_overdue}`,
    `открытые споры ${student.open_disputes}`,
    student.not_delivered ? "не доставлено" : "",
    ...student.warnings,
  ]
    .filter((part) => part !== "")
    .join(" ");
}

function readAttempt(value: unknown): AttemptRow | null {
  if (!isRecord(value)) {
    return null;
  }
  if (
    typeof value.attempt_id !== "string" ||
    typeof value.raw_input !== "string"
  ) {
    return null;
  }
  const verdict = value.verdict;
  const row: AttemptRow = {
    attempt_id: value.attempt_id,
    raw_input: value.raw_input,
  };
  if (verdict === "correct" || verdict === "partial" || verdict === "wrong") {
    row.verdict = verdict;
  }
  return row;
}

function byNextLesson(left: StudentSummary, right: StudentSummary): number {
  const leftAt = left.next_lesson_at ?? "";
  const rightAt = right.next_lesson_at ?? "";
  if (leftAt === rightAt) {
    return 0;
  }
  if (leftAt === "") {
    return 1;
  }
  if (rightAt === "") {
    return -1;
  }
  return leftAt < rightAt ? -1 : 1;
}

// Mantine рисует брейкпоинты тегом style внутри body. В тексте есть
// цифра ширины экрана, и она смешивается с сырым вводом попытки.
// Узел React не переносим: копируем правила в head и очищаем текст в body.
function parkProviderStyles(): void {
  const parked = new Set(
    [...document.head.querySelectorAll("style")].map(
      (node) => node.textContent ?? "",
    ),
  );
  for (const node of document.body.querySelectorAll("style")) {
    const text = node.textContent ?? "";
    if (text === "") {
      continue;
    }
    if (!parked.has(text)) {
      const copy = document.createElement("style");
      copy.textContent = text;
      document.head.appendChild(copy);
      parked.add(text);
    }
    node.textContent = "";
  }
}

function verdictLabel(verdict: Verdict): string {
  if (verdict === "correct") {
    return "верно";
  }
  if (verdict === "wrong") {
    return "неверно";
  }
  return "частично верно";
}

export function TutorPage() {
  useLayoutEffect(() => {
    parkProviderStyles();
  });

  const [telegramId, setTelegramId] = useState("");
  const [phase, setPhase] = useState<"login" | "offline" | "ready">("login");
  const [students, setStudents] = useState<StudentSummary[]>([]);
  const [attempts, setAttempts] = useState<AttemptRow[]>([]);

  async function loadStudents() {
    try {
      const list = await fetch("/students", { credentials: "include" });
      if (!list.ok) {
        return;
      }
      const payload: unknown = await list.json();
      if (!Array.isArray(payload)) {
        return;
      }
      const next = payload
        .map((item) => readStudent(item))
        .filter((item): item is StudentSummary => item !== null)
        .sort(byNextLesson);
      setAttempts([]);
      setStudents(next);
      setPhase("ready");
    } catch {
      setPhase("offline");
    }
  }

  async function onLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    let response: Response;
    try {
      response = await fetch("/auth/dev", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ telegram_id: telegramId, role: "tutor" }),
      });
    } catch {
      setPhase("offline");
      return;
    }
    rememberCsrfFromResponse(response);
    if (!response.ok) {
      return;
    }
    await loadStudents();
  }

  async function onOpen(student: StudentSummary) {
    const response = await fetch(
      `/assignments?student_id=${encodeURIComponent(student.id)}`,
      { credentials: "include" },
    );
    if (!response.ok) {
      return;
    }
    const payload: unknown = await response.json();
    if (!Array.isArray(payload)) {
      return;
    }
    const rows: AttemptRow[] = [];
    for (const item of payload) {
      if (!isRecord(item) || typeof item.id !== "string") {
        continue;
      }
      const attemptsResponse = await fetch(
        `/assignments/${encodeURIComponent(item.id)}/attempts`,
        { credentials: "include" },
      );
      if (!attemptsResponse.ok) {
        continue;
      }
      const body: unknown = await attemptsResponse.json();
      if (!Array.isArray(body)) {
        continue;
      }
      for (const attempt of body) {
        const row = readAttempt(attempt);
        if (row) {
          rows.push(row);
        }
      }
    }
    setAttempts(rows);
  }

  async function onLogout() {
    await fetch("/auth/logout", {
      method: "POST",
      credentials: "include",
      headers: csrfHeaders(),
    });
    setAttempts([]);
    setStudents([]);
    setPhase("login");
  }

  if (phase === "login") {
    return (
      <Screen title="Войти">
        <form onSubmit={(event) => void onLogin(event)}>
          <Stack gap="md">
            <TextInput
              label="telegram_id"
              name="telegram_id"
              autoComplete="off"
              value={telegramId}
              onChange={(event) => setTelegramId(event.currentTarget.value)}
            />
            <Button type="submit">Войти</Button>
          </Stack>
        </form>
      </Screen>
    );
  }

  return (
    <Screen title="Ученики">
      <Stack gap="md">
        <Button type="button" variant="default" onClick={() => void onLogout()}>
          Выход
        </Button>
        {phase === "offline" ? (
          <Stack gap="sm">
            <Text>Нет связи</Text>
            <Button
              type="button"
              variant="default"
              onClick={() => void loadStudents()}
            >
              Повторить
            </Button>
          </Stack>
        ) : (
          <List label="Ученики">
            {students.map((student) => (
              <ListButton
                key={student.id}
                title={student.display_name}
                subtitle={rowSubtitle(student)}
                onClick={() => void onOpen(student)}
              />
            ))}
          </List>
        )}
        {attempts.length > 0 ? (
          <Stack gap="sm" component="section">
            {attempts.map((attempt) => (
              <Text key={attempt.attempt_id}>
                {attempt.raw_input}
                {attempt.verdict ? ` ${verdictLabel(attempt.verdict)}` : ""}
              </Text>
            ))}
          </Stack>
        ) : null}
      </Stack>
    </Screen>
  );
}
