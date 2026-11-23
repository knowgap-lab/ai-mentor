import { Button, Stack, Text, Title } from "@mantine/core";
import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";
import { Screen } from "../ui";
import { errorSentence } from "./api-error";
import { isUnderEighteen, minskTime } from "./minsk-time";

type LessonRow = {
  id: string;
  placeNo: number;
  title: string;
  kind: string;
  topics: string[];
  time: string;
  week: string;
  skipped: boolean;
  moved: boolean;
  added: boolean;
};

type AttemptRow = {
  id: string;
  raw: string;
};

type Card = {
  name: string;
  status: string;
  consent: string;
  birthDate?: string;
  warnings: string[];
  lessons: LessonRow[];
  deadlines: string[];
  attempts: AttemptRow[];
  variants: string[];
};

type LinkView = {
  url: string;
  time: string;
};

type LoadResult =
  | { phase: "offline" }
  | { phase: "error"; sentence: string | null }
  | { phase: "ready"; card: Card };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function warningTexts(value: unknown): string[] {
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

function readStudent(value: unknown):
  | (Omit<Card, "lessons" | "deadlines" | "attempts" | "variants"> & {
      subjects: string[];
    })
  | null {
  if (!isRecord(value)) {
    return null;
  }
  if (typeof value.display_name !== "string") {
    return null;
  }
  if (typeof value.status !== "string" || typeof value.consent !== "string") {
    return null;
  }
  const subjects = Array.isArray(value.subjects)
    ? value.subjects.filter((item): item is string => typeof item === "string")
    : [];
  return {
    name: value.display_name,
    status: value.status,
    consent: value.consent,
    birthDate:
      typeof value.birth_date === "string" ? value.birth_date : undefined,
    warnings: warningTexts(value.warnings),
    subjects,
  };
}

function readLessons(value: unknown): LessonRow[] {
  if (!isRecord(value) || !Array.isArray(value.lessons)) {
    return [];
  }
  const lessons: LessonRow[] = [];
  for (const item of value.lessons) {
    if (!isRecord(item)) {
      continue;
    }
    if (typeof item.id !== "string" || typeof item.title !== "string") {
      continue;
    }
    if (typeof item.place_no !== "number") {
      continue;
    }
    const topics = Array.isArray(item.topics)
      ? item.topics.filter(
          (topic): topic is string => typeof topic === "string",
        )
      : [];
    lessons.push({
      id: item.id,
      placeNo: item.place_no,
      title: item.title,
      kind: typeof item.kind === "string" ? item.kind : "",
      topics,
      time: typeof item.starts_at === "string" ? minskTime(item.starts_at) : "",
      week: typeof item.planned_week === "string" ? item.planned_week : "",
      // Слова для planned, frozen и held в web.md нет.
      skipped: item.state === "skipped",
      moved: item.moved === true,
      added: item.added === true,
    });
  }
  return lessons;
}

function readAssignment(
  value: unknown,
): { id: string; deadline: string } | null {
  if (!isRecord(value) || typeof value.id !== "string") {
    return null;
  }
  return {
    id: value.id,
    deadline:
      typeof value.deadline === "string" ? minskTime(value.deadline) : "",
  };
}

function readAttempt(value: unknown): AttemptRow | null {
  if (!isRecord(value) || typeof value.raw_input !== "string") {
    return null;
  }
  const id =
    typeof value.attempt_id === "string" ? value.attempt_id : value.raw_input;
  return { id, raw: value.raw_input };
}

function readVariant(value: unknown): string | null {
  if (!isRecord(value) || typeof value.variant_id !== "string") {
    return null;
  }
  return value.variant_id;
}

function readLink(value: unknown): LinkView | null {
  if (!isRecord(value) || typeof value.url !== "string") {
    return null;
  }
  return {
    url: value.url,
    time:
      typeof value.expires_at === "string" ? minskTime(value.expires_at) : "",
  };
}

async function readPlans(
  studentId: string,
  subjects: string[],
): Promise<
  { ok: true; lessons: LessonRow[] } | { ok: false; sentence: string | null }
> {
  const lessons: LessonRow[] = [];
  for (const subject of subjects) {
    const response = await fetch(
      `/students/${encodeURIComponent(studentId)}/plans/${encodeURIComponent(subject)}`,
      { credentials: "include" },
    );
    if (!response.ok) {
      return { ok: false, sentence: await errorSentence(response) };
    }
    lessons.push(...readLessons(await response.json()));
  }
  return { ok: true, lessons };
}

async function readIssued(
  studentId: string,
): Promise<
  | { ok: true; deadlines: string[]; attempts: AttemptRow[] }
  | { ok: false; sentence: string | null }
> {
  const response = await fetch(
    `/assignments?student_id=${encodeURIComponent(studentId)}`,
    { credentials: "include" },
  );
  if (!response.ok) {
    return { ok: false, sentence: await errorSentence(response) };
  }
  const payload: unknown = await response.json();
  if (!Array.isArray(payload)) {
    return { ok: false, sentence: null };
  }
  const deadlines: string[] = [];
  const attempts: AttemptRow[] = [];
  for (const item of payload) {
    const assignment = readAssignment(item);
    if (!assignment) {
      continue;
    }
    if (assignment.deadline) {
      deadlines.push(assignment.deadline);
    }
    const attemptsResponse = await fetch(
      `/assignments/${encodeURIComponent(assignment.id)}/attempts`,
      { credentials: "include" },
    );
    if (!attemptsResponse.ok) {
      return { ok: false, sentence: await errorSentence(attemptsResponse) };
    }
    const body: unknown = await attemptsResponse.json();
    if (!Array.isArray(body)) {
      return { ok: false, sentence: null };
    }
    for (const attempt of body) {
      const row = readAttempt(attempt);
      if (row) {
        attempts.push(row);
      }
    }
  }
  return { ok: true, deadlines, attempts };
}

async function readDiscuss(
  studentId: string,
): Promise<
  { ok: true; variants: string[] } | { ok: false; sentence: string | null }
> {
  const response = await fetch(
    `/students/${encodeURIComponent(studentId)}/discuss-list`,
    { credentials: "include" },
  );
  if (!response.ok) {
    return { ok: false, sentence: await errorSentence(response) };
  }
  const payload: unknown = await response.json();
  if (!Array.isArray(payload)) {
    return { ok: false, sentence: null };
  }
  return {
    ok: true,
    variants: payload
      .map((item) => readVariant(item))
      .filter((item): item is string => item !== null),
  };
}

// Чтения правила расписания в контракте нет: время занятия берётся из starts_at.
async function loadCard(studentId: string): Promise<LoadResult> {
  try {
    const studentResponse = await fetch(
      `/students/${encodeURIComponent(studentId)}`,
      { credentials: "include" },
    );
    if (!studentResponse.ok) {
      return {
        phase: "error",
        sentence: await errorSentence(studentResponse),
      };
    }
    rememberCsrfFromResponse(studentResponse);
    const student = readStudent(await studentResponse.json());
    if (!student) {
      return { phase: "error", sentence: null };
    }
    const plans = await readPlans(studentId, student.subjects);
    if (!plans.ok) {
      return { phase: "error", sentence: plans.sentence };
    }
    const issued = await readIssued(studentId);
    if (!issued.ok) {
      return { phase: "error", sentence: issued.sentence };
    }
    const discuss = await readDiscuss(studentId);
    if (!discuss.ok) {
      return { phase: "error", sentence: discuss.sentence };
    }
    return {
      phase: "ready",
      card: {
        name: student.name,
        status: student.status,
        consent: student.consent,
        birthDate: student.birthDate,
        warnings: student.warnings,
        lessons: plans.lessons,
        deadlines: issued.deadlines,
        attempts: issued.attempts,
        variants: discuss.variants,
      },
    };
  } catch {
    return { phase: "offline" };
  }
}

async function postEmpty(path: string): Promise<Response> {
  return fetch(path, {
    method: "POST",
    credentials: "include",
    headers: csrfHeaders(),
  });
}

export function StudentCardPage() {
  const { studentId } = useParams();
  const [reloadKey, setReloadKey] = useState(0);
  const [phase, setPhase] = useState<"loading" | "offline" | "error" | "ready">(
    "loading",
  );
  const [errorText, setErrorText] = useState<string | null>(null);
  const [card, setCard] = useState<Card | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [invite, setInvite] = useState<LinkView | null>(null);
  const [consentLink, setConsentLink] = useState<LinkView | null>(null);

  useEffect(() => {
    if (!studentId) {
      return;
    }
    let cancelled = false;
    setPhase("loading");
    setActionError(null);
    setInvite(null);
    setConsentLink(null);
    void loadCard(studentId).then((result) => {
      if (cancelled) {
        return;
      }
      if (result.phase === "ready") {
        setCard(result.card);
        setPhase("ready");
        return;
      }
      setCard(null);
      if (result.phase === "offline") {
        setPhase("offline");
        return;
      }
      setErrorText(result.sentence);
      setPhase("error");
    });
    return () => {
      cancelled = true;
    };
  }, [studentId, reloadKey]);

  async function onArchive() {
    if (!studentId) {
      return;
    }
    setActionError(null);
    try {
      const response = await postEmpty(
        `/students/${encodeURIComponent(studentId)}/archive`,
      );
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      setCard((current) =>
        current ? { ...current, status: "archived" } : current,
      );
    } catch {
      setActionError("Нет связи");
    }
  }

  async function onUnarchive() {
    if (!studentId) {
      return;
    }
    setActionError(null);
    try {
      const response = await postEmpty(
        `/students/${encodeURIComponent(studentId)}/unarchive`,
      );
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      setCard((current) =>
        current ? { ...current, status: "active" } : current,
      );
    } catch {
      setActionError("Нет связи");
    }
  }

  async function onInvite() {
    if (!studentId) {
      return;
    }
    setActionError(null);
    try {
      const response = await postEmpty(
        `/students/${encodeURIComponent(studentId)}/invites`,
      );
      if (!response.ok) {
        setInvite(null);
        setActionError(await errorSentence(response));
        return;
      }
      setInvite(readLink(await response.json()));
    } catch {
      setActionError("Нет связи");
    }
  }

  async function onConsentLink() {
    if (!studentId || !card) {
      return;
    }
    setActionError(null);
    const kind =
      card.birthDate && isUnderEighteen(card.birthDate) ? "parent" : "student";
    try {
      const response = await fetch(
        `/students/${encodeURIComponent(studentId)}/consent-links`,
        {
          method: "POST",
          credentials: "include",
          headers: {
            "Content-Type": "application/json",
            ...csrfHeaders(),
          },
          body: JSON.stringify({ kind }),
        },
      );
      if (!response.ok) {
        setConsentLink(null);
        setActionError(await errorSentence(response));
        return;
      }
      setConsentLink(readLink(await response.json()));
    } catch {
      setActionError("Нет связи");
    }
  }

  async function onCancelRevoke() {
    if (!studentId) {
      return;
    }
    setActionError(null);
    try {
      const response = await postEmpty(
        `/students/${encodeURIComponent(studentId)}/consent/cancel-revoke`,
      );
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      // PV-R18: отзыв отменён, согласие снова действующее.
      setCard((current) =>
        current ? { ...current, consent: "given" } : current,
      );
    } catch {
      setActionError("Нет связи");
    }
  }

  return (
    <Screen title="Ученик">
      <Stack gap="md">
        {phase === "loading" ? <Text>Загрузка</Text> : null}
        {phase === "offline" ? (
          <Stack gap="sm">
            <Text>Нет связи</Text>
            <Button
              type="button"
              variant="default"
              onClick={() => setReloadKey((value) => value + 1)}
            >
              Повторить
            </Button>
          </Stack>
        ) : null}
        {phase === "error" && errorText ? <Text>{errorText}</Text> : null}
        {phase === "ready" && card ? (
          <>
            {card.warnings.map((warning) => (
              <Text key={warning}>{warning}</Text>
            ))}
            <Text>{card.name}</Text>
            <Title order={2} size="h3">
              Расписание
            </Title>
            {card.lessons.map((lesson) => (
              <Stack key={lesson.id} gap={0}>
                <Text>{lesson.title}</Text>
                <Text>{lesson.placeNo}</Text>
                {lesson.kind ? <Text>{lesson.kind}</Text> : null}
                {lesson.topics.map((topic) => (
                  <Text key={topic}>{topic}</Text>
                ))}
                {lesson.time ? <Text>{lesson.time}</Text> : null}
                {lesson.week ? <Text>{lesson.week}</Text> : null}
                {lesson.skipped ? <Text>пропущен</Text> : null}
                {lesson.moved ? <Text>перенесено</Text> : null}
                {lesson.added ? <Text>добавлено</Text> : null}
              </Stack>
            ))}
            <Title order={2} size="h3">
              Выдачи
            </Title>
            {card.deadlines.map((deadline) => (
              <Text key={deadline}>{deadline}</Text>
            ))}
            {card.attempts.map((attempt) => (
              <Text key={attempt.id}>{attempt.raw}</Text>
            ))}
            <Title order={2} size="h3">
              разобрать на уроке
            </Title>
            {card.variants.map((variant) => (
              <Text key={variant}>{variant}</Text>
            ))}
            <Title order={2} size="h3">
              Согласие
            </Title>
            {card.status === "archived" ? (
              <Button
                type="button"
                variant="default"
                onClick={() => void onUnarchive()}
              >
                Вернуть
              </Button>
            ) : (
              <Button
                type="button"
                variant="default"
                onClick={() => void onArchive()}
              >
                Архив
              </Button>
            )}
            <Button
              type="button"
              variant="default"
              onClick={() => void onInvite()}
            >
              Приглашение
            </Button>
            <Button
              type="button"
              variant="default"
              onClick={() => void onConsentLink()}
            >
              Ссылка согласия
            </Button>
            {card.consent === "revoked" ? (
              <Button
                type="button"
                variant="default"
                onClick={() => void onCancelRevoke()}
              >
                отменить отзыв
              </Button>
            ) : null}
            {actionError ? <Text>{actionError}</Text> : null}
            {invite ? <Text>{invite.url}</Text> : null}
            {invite?.time ? <Text>{invite.time}</Text> : null}
            {consentLink ? <Text>{consentLink.url}</Text> : null}
            {consentLink?.time ? <Text>{consentLink.time}</Text> : null}
          </>
        ) : null}
      </Stack>
    </Screen>
  );
}
