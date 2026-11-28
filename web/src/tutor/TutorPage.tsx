import { Button, Stack, Text, TextInput } from "@mantine/core";
import { type FormEvent, useState } from "react";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";
import { List, ListButton, Screen } from "../ui";
import { errorSentence } from "../api-error";
import { isRecord } from "../record";
import { minskTime } from "./minsk-time";

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

function readCount(value: unknown): number {
  return typeof value === "number" ? value : 0;
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

function readWarnings(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const out: string[] = [];
  for (const item of value) {
    if (isRecord(item) && typeof item.text === "string" && item.text) {
      out.push(item.text);
    }
  }
  return out;
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

export function TutorPage() {
  const [telegramId, setTelegramId] = useState("");
  const [phase, setPhase] = useState<"login" | "offline" | "ready">("login");
  const [loginNotice, setLoginNotice] = useState<string | null>(null);
  const [listNotice, setListNotice] = useState<string | null>(null);
  const [students, setStudents] = useState<StudentSummary[]>([]);
  const [attempts, setAttempts] = useState<string[]>([]);

  async function loadStudents() {
    try {
      const list = await fetch("/students", { credentials: "include" });
      if (!list.ok) {
        const sentence = await errorSentence(list);
        if (list.status === 401) {
          setLoginNotice(sentence ?? "Войдите ещё раз");
          setPhase("login");
          return;
        }
        setListNotice(sentence);
        setPhase("offline");
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
      setListNotice(null);
      setPhase("ready");
    } catch {
      setListNotice(null);
      setPhase("offline");
    }
  }

  async function onLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoginNotice(null);
    let response: Response;
    try {
      response = await fetch("/auth/dev", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ telegram_id: telegramId, role: "tutor" }),
      });
    } catch {
      setLoginNotice("Нет связи");
      return;
    }
    rememberCsrfFromResponse(response);
    if (!response.ok) {
      return;
    }
    await loadStudents();
  }

  async function onOpen(student: StudentSummary) {
    const response = await fetch(`/assignments?student_id=${student.id}`, {
      credentials: "include",
    });
    if (!response.ok) {
      return;
    }
    const payload: unknown = await response.json();
    if (!Array.isArray(payload)) {
      return;
    }
    const rows: string[] = [];
    for (const item of payload) {
      if (!isRecord(item) || typeof item.id !== "string") {
        continue;
      }
      const attemptsResponse = await fetch(`/assignments/${item.id}/attempts`, {
        credentials: "include",
      });
      if (!attemptsResponse.ok) {
        continue;
      }
      const body: unknown = await attemptsResponse.json();
      if (!Array.isArray(body)) {
        continue;
      }
      for (const attempt of body) {
        if (isRecord(attempt) && typeof attempt.raw_input === "string") {
          rows.push(attempt.raw_input);
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
    setLoginNotice(null);
    setListNotice(null);
    setPhase("login");
  }

  if (phase === "login") {
    return (
      <Screen title="Войти">
        <form onSubmit={(event) => void onLogin(event)}>
          <Stack gap="md">
            <TextInput
              label="telegram_id"
              value={telegramId}
              onChange={(event) => setTelegramId(event.currentTarget.value)}
            />
            <Button type="submit">Войти</Button>
            {loginNotice ? <Text>{loginNotice}</Text> : null}
          </Stack>
        </form>
      </Screen>
    );
  }

  return (
    <Screen title="Ученики">
      <Stack gap="md">
        <Button onClick={() => void onLogout()}>Выход</Button>
        {phase === "offline" ? (
          <Stack gap="sm">
            <Text>{listNotice ?? "Нет связи"}</Text>
            <Button onClick={() => void loadStudents()}>Повторить</Button>
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
        {attempts.length > 0
          ? attempts.map((raw) => <p key={raw}>{raw}</p>)
          : null}
      </Stack>
    </Screen>
  );
}

const HOME = [
  ["Алина К.", "ДЗ 6 из 8 · курс 58%"],
  ["Богдан М.", "ДЗ 3 из 5 · курс 41%"],
  ["Вера С.", "ДЗ 8 из 8 · курс 72%"],
  ["Глеб Т.", "ДЗ 1 из 6 · курс 22%"],
  ["Дина Л.", "ДЗ 4 из 7 · курс 49%"],
  ["Егор П.", "ДЗ 5 из 5 · курс 63%"],
] as const;

export function TutorHomePage() {
  const [dark, setDark] = useState(
    () => localStorage.getItem("am-th-scheme") === "dark",
  );
  return (
    <div
      className="tr"
      data-am-scheme={dark ? "dark" : "light"}
      data-testid="th"
    >
      <h1>Домашняя репетитора</h1>
      <label className="tt">
        <input
          type="checkbox"
          checked={dark}
          onChange={() => {
            const n = !dark;
            setDark(n);
            localStorage.setItem("am-th-scheme", n ? "dark" : "light");
          }}
          aria-label="Переключить светлую и тёмную тему"
        />
        <span>{dark ? "Тёмная тема" : "Светлая тема"}</span>
      </label>
      <div className="tp">
        <strong>Марина Волкова</strong>
        <p className="tm">Репетитор. План, ДЗ, разбор ошибок.</p>
      </div>
      <ul aria-label="Синтетические ученики">
        {HOME.map(([title, sub]) => (
          <li key={title}>
            <a href="/tutor">
              {title}
              <br />
              {sub}
            </a>
          </li>
        ))}
      </ul>
      <div className="tf">
        <ul className="ts">
          {["Telegram", "VK", "Сайт"].map((label) => (
            <li key={label}>
              <a href="/tutor">{label}</a>
            </li>
          ))}
        </ul>
        <a className="tc" href="/tutor">
          Войти в кабинет репетитора
        </a>
      </div>
    </div>
  );
}
