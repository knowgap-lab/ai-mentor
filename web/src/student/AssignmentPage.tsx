import { Button, Stack, Text } from "@mantine/core";
import { useEffect, useState } from "react";
import { useParams } from "react-router";
import {
  ActionBar,
  List,
  ListRow,
  Screen,
  StatusBadge,
  type Status,
} from "../ui";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";

type Verdict = "correct" | "partial" | "wrong" | "accepted";

type ItemRow = {
  item_id: string;
  statement: string;
  item_state: "open" | "final";
  last_verdict?: Verdict;
};

type Assignment = {
  id: string;
  subject?: string;
  items: ItemRow[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function readVerdict(value: unknown): Verdict | undefined {
  if (
    value === "correct" ||
    value === "partial" ||
    value === "wrong" ||
    value === "accepted"
  ) {
    return value;
  }
  return undefined;
}

function readItem(value: unknown): ItemRow | null {
  if (!isRecord(value)) {
    return null;
  }
  if (
    typeof value.item_id !== "string" ||
    typeof value.statement !== "string"
  ) {
    return null;
  }
  if (value.item_state !== "open" && value.item_state !== "final") {
    return null;
  }
  const verdict = readVerdict(value.last_verdict);
  return {
    item_id: value.item_id,
    statement: value.statement,
    item_state: value.item_state,
    last_verdict: verdict,
  };
}

function readAssignment(value: unknown, fallbackId: string): Assignment | null {
  if (!isRecord(value) || !Array.isArray(value.items)) {
    return null;
  }
  const items = value.items
    .map((item) => readItem(item))
    .filter((item): item is ItemRow => item !== null);
  return {
    id: typeof value.id === "string" ? value.id : fallbackId,
    subject: typeof value.subject === "string" ? value.subject : undefined,
    items,
  };
}

function statusState(item: ItemRow): Status {
  if (item.last_verdict === "correct") {
    return "correct";
  }
  if (item.last_verdict === "wrong") {
    return "wrong";
  }
  if (item.last_verdict === "partial") {
    return "partial";
  }
  if (item.item_state === "open") {
    return "not_answered";
  }
  return "closed";
}

function idempotencyKey(): string {
  if (typeof globalThis.crypto?.randomUUID === "function") {
    return globalThis.crypto.randomUUID();
  }
  return `idem-${Date.now().toString(16)}-${Math.random().toString(16).slice(2)}`;
}

function readLeft(value: unknown): number | null {
  if (!isRecord(value) || !isRecord(value.details)) {
    return null;
  }
  return typeof value.details.left === "number" ? value.details.left : null;
}

function readCode(value: unknown): string {
  if (!isRecord(value) || typeof value.code !== "string") {
    return "";
  }
  return value.code;
}

export function AssignmentPage() {
  const { assignmentId } = useParams();
  const [assignment, setAssignment] = useState<Assignment | null>(null);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const [submitted, setSubmitted] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  useEffect(() => {
    if (!assignmentId) {
      return;
    }
    let cancelled = false;
    async function load() {
      setLoading(true);
      setFailed(false);
      try {
        const response = await fetch(
          `/assignments/${encodeURIComponent(assignmentId ?? "")}`,
          { credentials: "include" },
        );
        rememberCsrfFromResponse(response);
        if (!response.ok) {
          throw new Error("load failed");
        }
        const payload: unknown = await response.json();
        const next = readAssignment(payload, assignmentId ?? "");
        if (!next) {
          throw new Error("load failed");
        }
        if (!cancelled) {
          setAssignment(next);
          setLoading(false);
        }
      } catch {
        if (!cancelled) {
          setAssignment(null);
          setFailed(true);
          setLoading(false);
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [assignmentId, reloadKey]);

  if (!assignmentId) {
    return (
      <Screen title="Выдача" backTo="/">
        <Text>Выдача не найдена</Text>
      </Screen>
    );
  }

  async function postSubmit(key: string, confirm: boolean): Promise<Response> {
    return fetch(
      `/assignments/${encodeURIComponent(assignmentId ?? "")}/submit`,
      {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
          "Idempotency-Key": key,
          ...csrfHeaders(),
        },
        body: JSON.stringify(confirm ? { confirm_unanswered: true } : {}),
      },
    );
  }

  async function onSubmit() {
    setSubmitError(null);
    const key = idempotencyKey();
    try {
      const response = await postSubmit(key, false);
      rememberCsrfFromResponse(response);
      if (response.ok) {
        setSubmitted(true);
        return;
      }
      if (response.status === 409) {
        const payload: unknown = await response.json();
        if (readCode(payload) === "unanswered_confirmation_required") {
          const left = readLeft(payload);
          if (left === null) {
            setSubmitError("Нет связи");
            return;
          }
          const agreed = window.confirm(`осталось ${left}`);
          if (!agreed) {
            return;
          }
          const again = await postSubmit(key, true);
          rememberCsrfFromResponse(again);
          if (again.ok) {
            setSubmitted(true);
            return;
          }
        }
      }
      setSubmitError("Нет связи");
    } catch {
      setSubmitError("Нет связи");
    }
  }

  const canSubmit = assignment !== null && !submitted;
  return (
    <Screen title="Выдача" backTo="/" withActionBar={canSubmit}>
      <Stack gap="md">
        {assignment?.subject ? <Text>{assignment.subject}</Text> : null}
        {loading ? <Text>Загрузка</Text> : null}
        {failed ? (
          <Stack gap="sm">
            <Text>Нет связи</Text>
            <Button
              variant="default"
              onClick={() => setReloadKey((value) => value + 1)}
            >
              Повторить
            </Button>
          </Stack>
        ) : null}
        {assignment && assignment.items.length === 0 ? (
          <Text>Пусто</Text>
        ) : null}
        {assignment && assignment.items.length > 0 ? (
          <List label="Задачи">
            {assignment.items.map((item) => (
              <ListRow
                key={item.item_id}
                to={`/task/${encodeURIComponent(assignment.id)}/${encodeURIComponent(item.item_id)}`}
                title={item.statement}
                aside={<StatusBadge state={statusState(item)} />}
              />
            ))}
          </List>
        ) : null}
        {submitted ? <Text>сдано</Text> : null}
        {submitError ? <Text>{submitError}</Text> : null}
      </Stack>
      {canSubmit ? (
        <ActionBar
          primary={{ label: "Сдать", onClick: () => void onSubmit() }}
        />
      ) : null}
    </Screen>
  );
}
