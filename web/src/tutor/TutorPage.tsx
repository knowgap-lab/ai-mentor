import { Button, Stack, Text, TextInput } from "@mantine/core";
import { type FormEvent, useEffect, useState } from "react";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";
import { List, ListRow, Screen } from "../ui";
import { errorSentence, isRecord } from "../api-error";
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

  async function onLogout() {
    await fetch("/auth/logout", {
      method: "POST",
      credentials: "include",
      headers: csrfHeaders(),
    });
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
        <Button onClick={() => void onLogout()}>
          Выход
        </Button>
        {phase === "offline" ? (
          <Stack gap="sm">
            <Text>{listNotice ?? "Нет связи"}</Text>
            <Button onClick={() => void loadStudents()}>
              Повторить
            </Button>
          </Stack>
        ) : (
          <List label="Ученики">
            {students.map((student) => (
              <ListRow
                key={student.id}
                to={`/tutor/students/${student.id}`}
                title={student.display_name}
                subtitle={rowSubtitle(student)}
              />
            ))}
          </List>
        )}
      </Stack>
    </Screen>
  );
}

export function TutorHomePage() {
  const [dark, setDark] = useState(
    () => localStorage.getItem("am-th-scheme") === "dark",
  );
  const [pack, setPack] = useState<{
    title: string;
    profileName: string;
    profileBio: string;
    listLabel: string;
    social: string[];
    cabinet: string;
    themeAria: string;
    themeDark: string;
    themeLight: string;
    rows: [string, string][];
  } | null>(null);
  useEffect(() => {
    fetch("/th.json").then((r) => r.json()).then(setPack);
  }, []);
  if (!pack) {
    return null;
  }
  return (
    <div
      className="tr"
      data-am-scheme={dark ? "dark" : "light"}
      data-testid="th"
    >
      <Screen title={pack.title}>
        <label className="tt">
          <input
            type="checkbox"
            checked={dark}
            onChange={() => {
              const n = !dark;
              setDark(n);
              localStorage.setItem("am-th-scheme", n ? "dark" : "light");
            }}
            aria-label={pack.themeAria}
          />
          <span>{dark ? pack.themeDark : pack.themeLight}</span>
        </label>
        <div className="tp">
          <strong>{pack.profileName}</strong>
          <p className="tm">{pack.profileBio}</p>
        </div>
        <List label={pack.listLabel}>
          {pack.rows.map(([t, sub]) => (
            <ListRow key={t} to="/tutor" title={t} subtitle={sub} />
          ))}
        </List>
        <div className="tf">
          <ul className="ts">
            {pack.social.map((label) => (
              <li key={label}>
                <a href="#" onClick={(e) => e.preventDefault()}>
                  {label}
                </a>
              </li>
            ))}
          </ul>
          <a className="tc" href="/tutor">
            {pack.cabinet}
          </a>
        </div>
      </Screen>
    </div>
  );
}