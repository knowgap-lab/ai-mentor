import { Button, Checkbox, Radio, TextInput } from "@mantine/core";
import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { csrfHeaders, rememberCsrfFromResponse } from "./csrf";
import { Statement } from "./Statement";
import "./task.css";

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

  if (!assignmentId || !itemId) {
    return (
      <main>
        <p>Задача не найдена</p>
      </main>
    );
  }

  if (loadError) {
    return (
      <main>
        <p>Нет связи</p>
        <Button
          type="button"
          onClick={() => setReloadKey((value) => value + 1)}
        >
          Повторить
        </Button>
      </main>
    );
  }

  if (!assignment) {
    return (
      <main>
        <p>Загрузка</p>
      </main>
    );
  }

  const item = assignment.items.find((entry) => entry.item_id === itemId);
  if (!item) {
    return (
      <main>
        <p>Задача не найдена</p>
      </main>
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
    setSendError(null);
    try {
      const response = await fetch(
        `/assignments/${encodeURIComponent(assignmentId ?? "")}/items/${encodeURIComponent(itemId ?? "")}/answer`,
        {
          method: "POST",
          credentials: "include",
          headers: {
            "Content-Type": "application/json",
            ...csrfHeaders(),
          },
          body: JSON.stringify({
            answer,
            idempotency_key: idempotencyKey(),
          }),
        },
      );
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        setSendError("Нет связи");
        return;
      }
      const body = (await response.json()) as AnswerResponse;
      setResult(body);
      if (body.hint) {
        setFormatHint(body.hint);
      }
    } catch {
      setSendError("Нет связи");
    }
  }

  function onAnswer() {
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
    <main>
      <Statement source={item.statement} />
      {attemptsLeft !== undefined ? <p>{attemptsLabel(attemptsLeft)}</p> : null}
      {item.answer_format === "ct.choice1" ? (
        <Radio.Group
          label="Вариант"
          name="choice1"
          value={choice ?? ""}
          onChange={setChoice}
        >
          {options.map((option) => (
            <Radio
              key={option.n}
              value={String(option.n)}
              label={`${option.n}. ${option.text}`}
            />
          ))}
        </Radio.Group>
      ) : null}
      {item.answer_format === "ct.choiceN" ? (
        <Checkbox.Group label="Варианты" value={choices} onChange={setChoices}>
          {options.map((option) => (
            <Checkbox
              key={option.n}
              value={String(option.n)}
              label={`${option.n}. ${option.text}`}
            />
          ))}
        </Checkbox.Group>
      ) : null}
      {item.answer_format === "ct.integer" ? (
        <div>
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
          <Button
            type="button"
            className="touch-target"
            variant="default"
            onClick={toggleMinus}
          >
            {MINUS}
          </Button>
        </div>
      ) : null}
      {formatHint ? <p>{formatHint}</p> : null}
      <Button
        type="button"
        className="touch-target"
        variant="default"
        onClick={onAnswer}
      >
        Ответить
      </Button>
      {verdict ? <p>{verdict}</p> : null}
      {result?.hint ? <p>{result.hint}</p> : null}
      {result && showScore(result) ? (
        <p>{`баллы ${result.points} из ${result.max_points}`}</p>
      ) : null}
      {result?.show?.key !== undefined ? <p>{result.show.key}</p> : null}
      {result?.show?.solution_block ? <p>решение</p> : null}
      {sendError ? <p>{sendError}</p> : null}
    </main>
  );
}
