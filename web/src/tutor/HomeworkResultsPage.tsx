import {
  Button,
  NativeSelect,
  Stack,
  Text,
  TextInput,
  Title,
} from "@mantine/core";
import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";
import { Screen, StatusBadge, type Status } from "../ui";
import { ERROR_TEXT, errorSentence, isRecord } from "../api-error";
import classes from "./field-focus.module.css";

// Причины спора — подписи AT-R18. Ключ и решение сюда не выводятся (WB-R4, WB-R22).

const REASON_LABEL: Record<string, string> = {
  key_wrong: "ключ неверен",
  statement_error: "ошибка в условии",
  equivalent_answer: "мой ответ эквивалентен",
  other: "другое",
};

const VERDICTS = [
  { value: "", label: "вердикт" },
  { value: "correct", label: "верно" },
  { value: "partial", label: "частично" },
  { value: "wrong", label: "неверно" },
] as const;

type Verdict = "correct" | "partial" | "wrong";

type ResultItem = {
  itemId: string;
  verdict: Verdict | null;
  points: number | null;
};

type Attempt = {
  id: string;
  itemId: string;
  raw: string;
  verdict: Verdict | null;
  points: number | null;
};

type Dispute = {
  id: string;
  itemId: string;
  reason: string;
  text: string;
  correctionId: number | null;
};

type ReportRow = {
  assignmentId: string;
  studentId: string;
};

type Report = {
  correctionId: number;
  rows: ReportRow[];
  skipped: string[];
};

type VerdictForm = {
  verdict: string;
  points: string;
  reason: string;
};

type Ready = {
  items: ResultItem[];
  attempts: Attempt[];
  disputes: Dispute[];
  reports: Report[];
  notSubmitted: boolean;
};

type LoadResult =
  | { phase: "offline" }
  | { phase: "error"; sentence: string | null }
  | ({ phase: "ready" } & Ready);

const emptyForm: VerdictForm = { verdict: "", points: "", reason: "" };

function readVerdict(value: unknown): Verdict | null {
  if (value === "correct" || value === "partial" || value === "wrong") {
    return value;
  }
  return null;
}

function readPoints(value: unknown): number | null {
  return typeof value === "number" ? value : null;
}

async function readFailure(
  response: Response,
): Promise<{ code: string | null; sentence: string | null }> {
  try {
    const payload: unknown = await response.json();
    if (!isRecord(payload) || typeof payload.code !== "string") {
      return { code: null, sentence: null };
    }
    return {
      code: payload.code,
      sentence: ERROR_TEXT[payload.code] ?? null,
    };
  } catch {
    return { code: null, sentence: null };
  }
}

function readItems(value: unknown): ResultItem[] | null {
  if (!isRecord(value) || !Array.isArray(value.items)) {
    return null;
  }
  const items: ResultItem[] = [];
  for (const entry of value.items) {
    if (!isRecord(entry) || typeof entry.item_id !== "string") {
      continue;
    }
    items.push({
      itemId: entry.item_id,
      verdict: readVerdict(entry.verdict),
      points: readPoints(entry.points),
    });
  }
  return items;
}

function readAttempts(value: unknown): Attempt[] | null {
  if (!Array.isArray(value)) {
    return null;
  }
  const attempts: Attempt[] = [];
  for (const entry of value) {
    if (!isRecord(entry) || typeof entry.attempt_id !== "string") {
      continue;
    }
    if (typeof entry.raw_input !== "string") {
      continue;
    }
    attempts.push({
      id: entry.attempt_id,
      itemId: typeof entry.item_id === "string" ? entry.item_id : "",
      raw: entry.raw_input,
      verdict: readVerdict(entry.verdict),
      points: readPoints(entry.points),
    });
  }
  return attempts;
}

function readDisputes(value: unknown, assignmentId: string): Dispute[] | null {
  if (!Array.isArray(value)) {
    return null;
  }
  const disputes: Dispute[] = [];
  for (const entry of value) {
    if (!isRecord(entry) || entry.assignment_id !== assignmentId) {
      continue;
    }
    if (typeof entry.id !== "string" || typeof entry.reason !== "string") {
      continue;
    }
    const correction = entry.correction_id;
    disputes.push({
      id: entry.id,
      itemId: typeof entry.item_id === "string" ? entry.item_id : "",
      reason: entry.reason,
      text: typeof entry.text === "string" ? entry.text : "",
      correctionId:
        typeof correction === "number" && correction >= 1 ? correction : null,
    });
  }
  return disputes;
}

function readReport(value: unknown): Report | null {
  if (!isRecord(value) || typeof value.correction_id !== "number") {
    return null;
  }
  const rows: ReportRow[] = [];
  if (Array.isArray(value.changed_assignments)) {
    for (const entry of value.changed_assignments) {
      if (!isRecord(entry) || typeof entry.assignment_id !== "string") {
        continue;
      }
      rows.push({
        assignmentId: entry.assignment_id,
        studentId: typeof entry.student_id === "string" ? entry.student_id : "",
      });
    }
  }
  const skipped = Array.isArray(value.skipped_manual_attempts)
    ? value.skipped_manual_attempts.filter(
        (item): item is string => typeof item === "string",
      )
    : [];
  return { correctionId: value.correction_id, rows, skipped };
}

async function loadResults(
  studentId: string,
  assignmentId: string,
): Promise<LoadResult> {
  try {
    const summaryUrl = `/assignments/${encodeURIComponent(assignmentId)}/summary`;
    const attemptsUrl = `/assignments/${encodeURIComponent(assignmentId)}/attempts`;
    const disputesUrl = `/students/${encodeURIComponent(studentId)}/disputes`;
    const [summaryRes, attemptsRes, disputesRes] = await Promise.all([
      fetch(summaryUrl, { credentials: "include" }),
      fetch(attemptsUrl, { credentials: "include" }),
      fetch(disputesUrl, { credentials: "include" }),
    ]);
    rememberCsrfFromResponse(summaryRes);
    rememberCsrfFromResponse(attemptsRes);
    rememberCsrfFromResponse(disputesRes);

    let notSubmitted = false;
    let items: ResultItem[] = [];
    if (!summaryRes.ok) {
      const failure = await readFailure(summaryRes);
      if (failure.code !== "not_submitted") {
        return { phase: "error", sentence: failure.sentence };
      }
      notSubmitted = true;
    } else {
      const parsed = readItems(await summaryRes.json());
      if (!parsed) {
        return { phase: "error", sentence: null };
      }
      items = parsed;
    }

    if (!attemptsRes.ok) {
      return { phase: "error", sentence: await errorSentence(attemptsRes) };
    }
    const attempts = readAttempts(await attemptsRes.json());
    if (!attempts) {
      return { phase: "error", sentence: null };
    }

    if (!disputesRes.ok) {
      return { phase: "error", sentence: await errorSentence(disputesRes) };
    }
    const disputes = readDisputes(await disputesRes.json(), assignmentId);
    if (!disputes) {
      return { phase: "error", sentence: null };
    }

    const reports: Report[] = [];
    const seen = new Set<number>();
    for (const dispute of disputes) {
      if (dispute.correctionId === null || seen.has(dispute.correctionId)) {
        continue;
      }
      seen.add(dispute.correctionId);
      const response = await fetch(`/rechecks/${dispute.correctionId}`, {
        credentials: "include",
      });
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        return { phase: "error", sentence: await errorSentence(response) };
      }
      const report = readReport(await response.json());
      if (!report) {
        return { phase: "error", sentence: null };
      }
      reports.push(report);
    }
    return {
      phase: "ready",
      items,
      attempts,
      disputes,
      reports,
      notSubmitted,
    };
  } catch {
    return { phase: "offline" };
  }
}

function badge(verdict: Verdict | null) {
  if (!verdict) {
    return null;
  }
  const state: Status = verdict;
  return <StatusBadge state={state} />;
}

export function HomeworkResultsPage() {
  const { studentId, assignmentId } = useParams();
  const [reloadKey, setReloadKey] = useState(0);
  const [phase, setPhase] = useState<"loading" | "offline" | "error" | "ready">(
    "loading",
  );
  const [errorText, setErrorText] = useState<string | null>(null);
  const [ready, setReady] = useState<Ready | null>(null);
  const [forms, setForms] = useState<Record<string, VerdictForm>>({});
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    if (!studentId || !assignmentId) {
      return;
    }
    let cancelled = false;
    setPhase("loading");
    setActionError(null);
    setForms({});
    void loadResults(studentId, assignmentId).then((result) => {
      if (cancelled) {
        return;
      }
      if (result.phase === "ready") {
        setReady({
          items: result.items,
          attempts: result.attempts,
          disputes: result.disputes,
          reports: result.reports,
          notSubmitted: result.notSubmitted,
        });
        setPhase("ready");
        return;
      }
      setReady(null);
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
  }, [studentId, assignmentId, reloadKey]);

  function formOf(attemptId: string): VerdictForm {
    return forms[attemptId] ?? emptyForm;
  }

  function setField(attemptId: string, patch: Partial<VerdictForm>) {
    setForms((current) => ({
      ...current,
      [attemptId]: { ...emptyForm, ...current[attemptId], ...patch },
    }));
  }

  async function onVerdict(attempt: Attempt) {
    const form = formOf(attempt.id);
    const points = Number(form.points);
    if (
      form.verdict !== "correct" &&
      form.verdict !== "partial" &&
      form.verdict !== "wrong"
    ) {
      return;
    }
    if (!Number.isInteger(points) || points < 0) {
      return;
    }
    setActionError(null);
    try {
      const response = await fetch(
        `/attempts/${encodeURIComponent(attempt.id)}/manual-verdict`,
        {
          method: "POST",
          credentials: "include",
          headers: {
            "Content-Type": "application/json",
            ...csrfHeaders(),
          },
          body: JSON.stringify({
            verdict: form.verdict,
            points,
            reason: form.reason,
          }),
        },
      );
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      const body: unknown = await response.json();
      const verdict = isRecord(body) ? readVerdict(body.verdict) : form.verdict;
      const nextPoints = isRecord(body) ? readPoints(body.points) : points;
      setReady((current) =>
        current
          ? {
              ...current,
              attempts: current.attempts.map((item) =>
                item.id === attempt.id
                  ? {
                      ...item,
                      verdict,
                      points: nextPoints ?? points,
                    }
                  : item,
              ),
            }
          : current,
      );
    } catch {
      setActionError("Нет связи");
    }
  }

  return (
    <Screen title="Итоги ДЗ">
      <Stack gap="md" style={{ minWidth: 0, overflowWrap: "anywhere" }}>
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
        {phase === "ready" && ready ? (
          <>
            <Title order={2} size="h3">
              результаты по задачам
            </Title>
            {ready.items.map((item) => (
              <Stack key={item.itemId} gap={0}>
                <Text>{item.itemId}</Text>
                {badge(item.verdict)}
                {item.points !== null ? <Text>{item.points}</Text> : null}
              </Stack>
            ))}
            {ready.attempts.map((attempt) => {
              const form = formOf(attempt.id);
              return (
                <Stack key={attempt.id} gap="sm">
                  <Text>{attempt.raw}</Text>
                  {attempt.itemId ? <Text>{attempt.itemId}</Text> : null}
                  {ready.notSubmitted ? <Text>ещё не сдано</Text> : null}
                  {badge(attempt.verdict)}
                  <NativeSelect
                    aria-label="вердикт"
                    classNames={{ input: classes.field }}
                    size="md"
                    value={form.verdict}
                    data={[...VERDICTS]}
                    onChange={(event) =>
                      setField(attempt.id, {
                        verdict: event.currentTarget.value,
                      })
                    }
                  />
                  <TextInput
                    aria-label="баллы"
                    classNames={{ input: classes.field }}
                    value={form.points}
                    onChange={(event) =>
                      setField(attempt.id, {
                        points: event.currentTarget.value,
                      })
                    }
                  />
                  <TextInput
                    aria-label="причина"
                    classNames={{ input: classes.field }}
                    value={form.reason}
                    onChange={(event) =>
                      setField(attempt.id, {
                        reason: event.currentTarget.value,
                      })
                    }
                  />
                  <Button
                    type="button"
                    variant="default"
                    onClick={() => void onVerdict(attempt)}
                  >
                    ручной вердикт
                  </Button>
                </Stack>
              );
            })}
            <Title order={2} size="h3">
              споры
            </Title>
            {ready.disputes.map((dispute) => (
              <Stack key={dispute.id} gap={0}>
                {REASON_LABEL[dispute.reason] ? (
                  <Text>{REASON_LABEL[dispute.reason]}</Text>
                ) : null}
                {dispute.text ? <Text>{dispute.text}</Text> : null}
                {dispute.itemId ? <Text>{dispute.itemId}</Text> : null}
              </Stack>
            ))}
            <Title order={2} size="h3">
              отчёт перепроверки
            </Title>
            {ready.reports.map((report) => (
              <Stack key={report.correctionId} gap={0}>
                {report.rows.map((row) => (
                  <Stack key={row.assignmentId} gap={0}>
                    <Text>{row.assignmentId}</Text>
                    {row.studentId ? <Text>{row.studentId}</Text> : null}
                  </Stack>
                ))}
                {report.skipped.map((attemptId) => (
                  <Text key={attemptId}>{attemptId}</Text>
                ))}
              </Stack>
            ))}
            {actionError ? <Text>{actionError}</Text> : null}
          </>
        ) : null}
      </Stack>
    </Screen>
  );
}
