import { Button, NativeSelect, Stack, Text, Textarea } from "@mantine/core";
import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { messageForResponse, OFFLINE_TEXT } from "../api-error";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";
import { Screen, StatusBadge, type Status } from "../ui";

const REASONS = [
  { value: "key_wrong", label: "ключ неверен" },
  { value: "statement_error", label: "ошибка в условии" },
  { value: "equivalent_answer", label: "мой ответ эквивалентен" },
  { value: "other", label: "другое" },
] as const;

type Verdict = "correct" | "partial" | "wrong";

type SummaryItem = {
  itemId: string;
  verdict: Verdict | null;
  showKey: string | null;
  hasSolution: boolean;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function readVerdict(value: unknown): Verdict | null {
  if (value === "correct" || value === "partial" || value === "wrong") {
    return value;
  }
  return null;
}

function readItem(value: unknown): SummaryItem | null {
  if (!isRecord(value) || typeof value.item_id !== "string") {
    return null;
  }
  const show = isRecord(value.show) ? value.show : null;
  const showKey = show && typeof show.key === "string" ? show.key : null;
  const hasSolution =
    show !== null &&
    typeof show.solution_block === "string" &&
    show.solution_block !== "";
  return {
    itemId: value.item_id,
    verdict: readVerdict(value.verdict),
    showKey,
    hasSolution,
  };
}

function readItems(value: unknown): SummaryItem[] | null {
  if (!isRecord(value) || !Array.isArray(value.items)) {
    return null;
  }
  return value.items
    .map((item) => readItem(item))
    .filter((item): item is SummaryItem => item !== null);
}

export function SummaryPage() {
  const { assignmentId } = useParams();
  const [items, setItems] = useState<SummaryItem[] | null>(null);
  const [offline, setOffline] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [reason, setReason] = useState<string>(REASONS[0].value);
  const [note, setNote] = useState("");

  useEffect(() => {
    if (!assignmentId) {
      return;
    }
    let cancelled = false;
    setOffline(false);
    setLoadError(null);
    async function load() {
      try {
        const response = await fetch(
          `/assignments/${encodeURIComponent(assignmentId ?? "")}/summary`,
          { credentials: "include" },
        );
        rememberCsrfFromResponse(response);
        if (!response.ok) {
          const text = await messageForResponse(response);
          if (!cancelled) {
            setItems(null);
            setLoadError(text);
          }
          return;
        }
        const body = (await response.json()) as unknown;
        const next = readItems(body);
        if (!cancelled) {
          setItems(next ?? []);
          setLoadError(null);
        }
      } catch {
        if (!cancelled) {
          setItems(null);
          setLoadError(null);
          setOffline(true);
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [assignmentId, reloadKey]);

  async function post(
    itemId: string,
    path: "did-not-understand" | "dispute",
    body?: string,
  ) {
    if (!assignmentId) {
      return;
    }
    setActionError(null);
    try {
      const response = await fetch(
        `/assignments/${encodeURIComponent(assignmentId)}/items/${encodeURIComponent(itemId)}/${path}`,
        {
          method: "POST",
          credentials: "include",
          headers: {
            ...csrfHeaders(),
            ...(body !== undefined
              ? { "Content-Type": "application/json" }
              : {}),
          },
          body,
        },
      );
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        setActionError(await messageForResponse(response));
      }
    } catch {
      setOffline(true);
    }
  }

  function dispute(itemId: string) {
    const payload: { reason: string; text?: string } = { reason };
    if (note !== "") {
      payload.text = note;
    }
    void post(itemId, "dispute", JSON.stringify(payload));
  }

  const showForm = items !== null && items.length > 0;

  return (
    <Screen
      title="Итог"
      backTo={assignmentId ? `/assignment/${assignmentId}` : "/"}
    >
      <Stack gap="md">
        {offline ? (
          <Stack gap="sm">
            <Text>{OFFLINE_TEXT}</Text>
            <Button
              type="button"
              variant="default"
              onClick={() => setReloadKey((value) => value + 1)}
            >
              Повторить
            </Button>
          </Stack>
        ) : null}
        {loadError ? <Text>{loadError}</Text> : null}
        {items?.map((item) => (
          <Stack key={item.itemId} gap="sm">
            {" "}
            {item.verdict ? (
              <StatusBadge state={item.verdict as Status} />
            ) : null}{" "}
            {item.showKey ? <Text>{item.showKey}</Text> : null}{" "}
            {item.hasSolution ? <Text>решение</Text> : null}{" "}
            <Button
              type="button"
              variant="default"
              onClick={() => void post(item.itemId, "did-not-understand")}
            >
              не понял
            </Button>
            <Button type="button" onClick={() => dispute(item.itemId)}>
              оспорить
            </Button>{" "}
          </Stack>
        ))}
        {showForm ? (
          <Stack gap="sm">
            <NativeSelect
              aria-label="Причина"
              size="md"
              value={reason}
              data={REASONS.map((item) => ({
                value: item.value,
                label: item.label,
              }))}
              onChange={(event) => setReason(event.currentTarget.value)}
            />
            <Textarea
              aria-label="Текст"
              size="md"
              value={note}
              onChange={(event) =>
                setNote(event.currentTarget.value.slice(0, 300))
              }
            />
          </Stack>
        ) : null}
        {actionError ? <Text>{actionError}</Text> : null}
      </Stack>
    </Screen>
  );
}
