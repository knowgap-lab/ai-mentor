import { Button, Checkbox, Radio, Stack, Text, TextInput } from "@mantine/core";
import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router";
import { ActionBar, ChoiceRow, Screen, TaskSheet } from "../ui";
import { csrfHeaders, rememberCsrfFromResponse } from "./csrf";
import {
  dropOutbox,
  readOutbox,
  upsertOutbox,
  type OutboxItem,
} from "./outbox";
import { Statement } from "./Statement";

const INTEGER_HINT = "введите целое число";
const MINUS = "−";

type Option = {
  n: number;
  text: string;
};

type ItemState = {
  item_id: string;
  statement: string;
  answer_format: string;
  item_state: "open" | "final";
  attempts_left?: number;
  options?: Option[];
};

type AssignmentStudentView = {
  id: string;
  kind: string;
  state: string;
  items: ItemState[];
};

type AnswerShow = {
  key?: string;
  solution_block?: string;
};

type AnswerResponse = {
  status: "malformed" | "accepted" | "scored";
  hint?: string;
  item_state: "open" | "final";
  attempts_left?: number;
  verdict?: "correct" | "partial" | "wrong";
  points?: number;
  max_points?: number;
  show?: AnswerShow;
};

function idempotencyKey(): string {
  if (typeof globalThis.crypto?.randomUUID === "function") {
    return globalThis.crypto.randomUUID();
  }
  return `idem-${Date.now().toString(16)}-${Math.random().toString(16).slice(2)}`;
}

function normalizeInteger(value: string): string | null {
  const compact = value.replace(/[\s\u00a0]/g, "").replace(/[−–‐]/g, "-");
  if (compact.includes(",") || compact.includes(".")) {
    return null;
  }
  const unsigned = compact.startsWith("+") ? compact.slice(1) : compact;
  if (!/^-?\d+$/.test(unsigned)) {
    return null;
  }
  return unsigned;
}

function integerHint(value: string): string | null {
  if (value.includes(",") || value.includes(".")) {
    return INTEGER_HINT;
  }
  const trimmed = value.trim();
  if (
    trimmed === "" ||
    trimmed === "-" ||
    trimmed === MINUS ||
    trimmed === "+"
  ) {
    return null;
  }
  if (normalizeInteger(value) === null) {
    return INTEGER_HINT;
  }
  return null;
}

function itemOptions(item: ItemState): Option[] {
  const given = item.options ?? [];
  if (given.length > 0) {
    return [...given].sort((a, b) => a.n - b.n);
  }
  if (
    item.answer_format === "ct.choice1" ||
    item.answer_format === "ct.choiceN"
  ) {
    return [1, 2, 3, 4, 5].map((n) => ({ n, text: String(n) }));
  }
  return [];
}

function attemptsLabel(left: number): string {
  const mod10 = left % 10;
  const mod100 = left % 100;
  if (mod10 === 1 && mod100 !== 11) {
    return `осталась ${left} попытка`;
  }
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) {
    return `осталось ${left} попытки`;
  }
  return `осталось ${left} попыток`;
}

function verdictText(result: AnswerResponse): string | null {
  if (result.verdict === "correct") {
    return "верно";
  }
  if (result.verdict === "wrong") {
    return "неверно";
  }
  if (result.verdict === "partial") {
    return "частично верно";
  }
  if (result.status === "accepted") {
    return "ответ принят";
  }
  return null;
}

type Delivered =
  { kind: "ok"; body: AnswerResponse } | { kind: "http" } | { kind: "offline" };

async function deliver(entry: OutboxItem): Promise<Delivered> {
  try {
    const response = await fetch(
      `/assignments/${encodeURIComponent(entry.assignmentId)}/items/${encodeURIComponent(entry.itemId)}/answer`,
      {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
          ...csrfHeaders(),
        },
        body: JSON.stringify({
          answer: entry.answer,
          idempotency_key: entry.idempotency_key,
        }),
      },
    );
    rememberCsrfFromResponse(response);
    if (!response.ok) {
      return { kind: "http" };
    }
    const body = (await response.json()) as AnswerResponse;
    return { kind: "ok", body };
  } catch {
    return { kind: "offline" };
  }
}

function showScore(result: AnswerResponse): boolean {
  if (result.points === undefined || result.max_points === undefined) {
    return false;
  }
  if (result.show?.key !== undefined || result.show?.solution_block) {
    return true;
  }
  if (result.verdict === "wrong" && result.item_state === "open") {
    return false;
  }
  return result.verdict !== undefined;
}

export function TaskPage() {
  const { assignmentId, itemId } = useParams();
  const [assignment, setAssignment] = useState<AssignmentStudentView | null>(
    null,
  );
  const [loadError, setLoadError] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const [choice, setChoice] = useState<string | null>(null);
  const [choices, setChoices] = useState<string[]>([]);
  const [draft, setDraft] = useState("");
  const [formatHint, setFormatHint] = useState<string | null>(null);
  const [result, setResult] = useState<AnswerResponse | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);
  const [unsent, setUnsent] = useState(false);
  const blocked = useRef(false);
  const flushing = useRef(false);

  useEffect(() => {
    if (!assignmentId) {
      return;
    }
    let cancelled = false;
    setLoadError(false);
    async function load() {
      try {
        const response = await fetch(
          `/assignments/${encodeURIComponent(assignmentId ?? "")}`,
          {
            credentials: "include",
          },
        );
        rememberCsrfFromResponse(response);
        if (!response.ok) {
          throw new Error("load failed");
        }
        const body = (await response.json()) as AssignmentStudentView;
        if (!cancelled) {
          setAssignment(body);
        }
      } catch {
        if (!cancelled) {
          setAssignment(null);
          setLoadError(true);
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [assignmentId, reloadKey]);

  useEffect(() => {
    if (!assignmentId || !itemId) {
      return;
    }
    let cancelled = false;
    const queued = readOutbox().some(
      (entry) => entry.assignmentId === assignmentId && entry.itemId === itemId,
    );
    setUnsent(queued);
    blocked.current = queued;

    async function flush() {
      if (!assignmentId || !itemId || flushing.current) {
        return;
      }
      const entry = readOutbox().find(
        (item) => item.assignmentId === assignmentId && item.itemId === itemId,
      );
      if (!entry || navigator.onLine === false) {
        return;
      }
      flushing.current = true;
      dropOutbox(assignmentId, itemId);
      try {
        const outcome = await deliver(entry);
        if (outcome.kind === "offline") {
          upsertOutbox(entry);
          if (!cancelled) {
            setUnsent(true);
          }
          return;
        }
        blocked.current = false;
        if (cancelled) {
          return;
        }
        if (outcome.kind === "http") {
          setUnsent(false);
          setSendError("Нет связи");
          return;
        }
        setUnsent(false);
        setResult(outcome.body);
        if (outcome.body.hint) {
          setFormatHint(outcome.body.hint);
        }
      } finally {
        flushing.current = false;
      }
    }

    function onOnline() {
      void flush();
    }
    window.addEventListener("online", onOnline);
    void flush();
    return () => {
      cancelled = true;
      window.removeEventListener("online", onOnline);
    };
  }, [assignmentId, itemId]);

  const backTo =
    assignmentId && itemId
      ? `/assignment/${encodeURIComponent(assignmentId)}`
      : "/";

  if (!assignmentId || !itemId) {
    return (
      <Screen title="Задача" backTo="/">
        <Text>Задача не найдена</Text>
      </Screen>
    );
  }

  if (loadError) {
    return (
      <Screen title="Задача" backTo={backTo}>
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
      </Screen>
    );
  }

  if (!assignment) {
    return (
      <Screen title="Задача" backTo={backTo}>
        <Text>Загрузка</Text>
      </Screen>
    );
  }

  const item = assignment.items.find((entry) => entry.item_id === itemId);
  if (!item) {
    return (
      <Screen title="Задача" backTo={backTo}>
        <Text>Задача не найдена</Text>
      </Screen>
    );
  }

  const attemptsLeft =
    result && typeof result.attempts_left === "number"
      ? result.attempts_left
      : item.attempts_left;

  function updateDraft(next: string) {
    setDraft(next);
    setFormatHint(integerHint(next));
  }

  function toggleMinus() {
    const normalized = draft.replace(/^[−–‐]/, "-");
    const next = normalized.startsWith("-")
      ? normalized.slice(1)
      : `-${normalized}`;
    updateDraft(next);
  }

  function answerPayload(): string | number[] | null {
    if (item?.answer_format === "ct.choice1") {
      if (choice === null) {
        return null;
      }
      const n = Number(choice);
      return Number.isInteger(n) ? [n] : null;
    }
    if (item?.answer_format === "ct.choiceN") {
      const numbers = choices
        .map((value) => Number(value))
        .filter((value) => Number.isInteger(value))
        .sort((a, b) => a - b);
      return numbers.length > 0 ? numbers : null;
    }
    if (item?.answer_format === "ct.integer") {
      return normalizeInteger(draft);
    }
    return null;
  }

  async function send(answer: string | number[]) {
    if (!assignmentId || !itemId || blocked.current) {
      return;
    }
    setSendError(null);
    const entry: OutboxItem = {
      assignmentId,
      itemId,
      answer,
      idempotency_key: idempotencyKey(),
    };
    if (navigator.onLine === false) {
      blocked.current = true;
      upsertOutbox(entry);
      setUnsent(true);
      return;
    }
    blocked.current = true;
    const outcome = await deliver(entry);
    if (outcome.kind === "offline") {
      upsertOutbox(entry);
      setUnsent(true);
      return;
    }
    blocked.current = false;
    if (outcome.kind === "http") {
      setSendError("Нет связи");
      return;
    }
    setResult(outcome.body);
    if (outcome.body.hint) {
      setFormatHint(outcome.body.hint);
    }
  }

  function onAnswer() {
    if (unsent || blocked.current) {
      return;
    }
    const answer = answerPayload();
    if (answer === null) {
      if (item?.answer_format === "ct.integer") {
        setFormatHint(INTEGER_HINT);
      }
      return;
    }
    void send(answer);
  }

  const options = itemOptions(item);
  const verdict = result ? verdictText(result) : null;

  return (
    <Screen title="Задача" backTo={backTo} withActionBar>
      <TaskSheet>
        <Stack gap="md">
          <Statement source={item.statement} />
          {attemptsLeft !== undefined ? (
            <Text>{attemptsLabel(attemptsLeft)}</Text>
          ) : null}
          {item.answer_format === "ct.choice1" ? (
            <Radio.Group
              label="Вариант"
              name="choice1"
              size="md"
              value={choice ?? ""}
              onChange={setChoice}
            >
              <Stack gap="sm">
                {options.map((option) => (
                  <ChoiceRow key={option.n}>
                    <Radio
                      value={String(option.n)}
                      label={`${option.n}. ${option.text}`}
                    />
                  </ChoiceRow>
                ))}
              </Stack>
            </Radio.Group>
          ) : null}
          {item.answer_format === "ct.choiceN" ? (
            <Checkbox.Group
              label="Варианты"
              size="md"
              value={choices}
              onChange={setChoices}
            >
              <Stack gap="sm">
                {options.map((option) => (
                  <ChoiceRow key={option.n}>
                    <Checkbox
                      value={String(option.n)}
                      label={`${option.n}. ${option.text}`}
                    />
                  </ChoiceRow>
                ))}
              </Stack>
            </Checkbox.Group>
          ) : null}
          {item.answer_format === "ct.integer" ? (
            <Stack gap="sm">
              <TextInput
                label="Ответ"
                inputMode="numeric"
                autoComplete="off"
                value={draft}
                onChange={(event) => updateDraft(event.currentTarget.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    onAnswer();
                  }
                }}
              />
              <Button type="button" variant="default" onClick={toggleMinus}>
                {MINUS}
              </Button>
            </Stack>
          ) : null}
          {formatHint ? <Text>{formatHint}</Text> : null}
          {unsent ? <Text>не отправлено</Text> : null}
          {unsent && item.answer_format === "ct.integer" ? (
            <Text>ждём связь</Text>
          ) : null}
          {verdict ? <Text>{verdict}</Text> : null}
          {result?.hint ? <Text>{result.hint}</Text> : null}
          {result && showScore(result) ? (
            <Text className="am-num">{`баллы ${result.points} из ${result.max_points}`}</Text>
          ) : null}
          {result?.show?.key !== undefined ? (
            <Text>{result.show.key}</Text>
          ) : null}
          {result?.show?.solution_block ? <Text>решение</Text> : null}
          {sendError ? <Text>{sendError}</Text> : null}
        </Stack>
      </TaskSheet>
      <ActionBar
        primary={{
          label: "Ответить",
          onClick: onAnswer,
          disabled: unsent,
        }}
      />
    </Screen>
  );
}
