import { Button, Stack, Text, TextInput } from "@mantine/core";
import { useEffect, useState } from "react";
import { useParams, useSearchParams } from "react-router";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";
import { ActionBar, Screen } from "../ui";
import { errorSentence, isRecord } from "../api-error";
import classes from "./field-focus.module.css";
import { minskLocalToUtc, minskTime } from "./minsk-time";

// HomeworkDraft не содержит id выдачи. После «Выдать» он берётся из ответа.
// При повторном открытии адрес может передать ?assignment=: пустые попытки
// включают «Отозвать», непустые выключают (WB-R23, AT-R6).

type DraftItem = {
  variantId: string;
  statement: string;
};

type Draft = {
  deadline: string;
  items: DraftItem[];
  hints: string[];
  warnings: string[];
  issued: boolean;
};

type LoadResult =
  | { phase: "offline" }
  | { phase: "error"; sentence: string | null }
  | {
      phase: "ready";
      draft: Draft;
      assignmentId: string | null;
      canWithdraw: boolean | null;
    };

function strings(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string");
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

function readDraft(value: unknown): Draft | null {
  if (!isRecord(value) || typeof value.issued !== "boolean") {
    return null;
  }
  if (!Array.isArray(value.items)) {
    return null;
  }
  const items: DraftItem[] = [];
  for (const entry of value.items) {
    if (!isRecord(entry) || typeof entry.variant_id !== "string") {
      continue;
    }
    items.push({
      variantId: entry.variant_id,
      statement: typeof entry.statement === "string" ? entry.statement : "",
    });
  }
  return {
    deadline: typeof value.deadline === "string" ? value.deadline : "",
    items,
    hints: strings(value.hints),
    warnings: warningTexts(value.warnings),
    issued: value.issued,
  };
}

function readTutorView(
  value: unknown,
): { id: string; canWithdraw: boolean } | null {
  if (!isRecord(value) || typeof value.id !== "string") {
    return null;
  }
  return { id: value.id, canWithdraw: value.can_withdraw === true };
}

async function loadDraft(
  lessonId: string,
  assignmentId: string,
): Promise<LoadResult> {
  try {
    const response = await fetch(
      `/lessons/${encodeURIComponent(lessonId)}/homework-draft`,
      { credentials: "include" },
    );
    rememberCsrfFromResponse(response);
    if (!response.ok) {
      return { phase: "error", sentence: await errorSentence(response) };
    }
    const draft = readDraft(await response.json());
    if (!draft) {
      return { phase: "error", sentence: null };
    }
    if (!draft.issued || assignmentId === "") {
      return { phase: "ready", draft, assignmentId: null, canWithdraw: null };
    }
    const attempts = await fetch(
      `/assignments/${encodeURIComponent(assignmentId)}/attempts`,
      { credentials: "include" },
    );
    rememberCsrfFromResponse(attempts);
    if (!attempts.ok) {
      return { phase: "error", sentence: await errorSentence(attempts) };
    }
    const body: unknown = await attempts.json();
    if (!Array.isArray(body)) {
      return { phase: "error", sentence: null };
    }
    return {
      phase: "ready",
      draft,
      assignmentId,
      canWithdraw: body.length === 0,
    };
  } catch {
    return { phase: "offline" };
  }
}

function draftUrl(lessonId: string, action?: string): string {
  const base = `/lessons/${encodeURIComponent(lessonId)}/homework-draft`;
  return action ? `${base}/${action}` : base;
}

async function sendJson(
  path: string,
  method: "PATCH" | "POST",
  body?: unknown,
): Promise<Response> {
  return fetch(path, {
    method,
    credentials: "include",
    headers: {
      ...csrfHeaders(),
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export function HomeworkDraftPage() {
  const { lessonId } = useParams();
  const [params] = useSearchParams();
  const assignmentFromUrl = params.get("assignment") ?? "";
  const [reloadKey, setReloadKey] = useState(0);
  const [phase, setPhase] = useState<"loading" | "offline" | "error" | "ready">(
    "loading",
  );
  const [errorText, setErrorText] = useState<string | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [assignmentId, setAssignmentId] = useState<string | null>(null);
  const [canWithdraw, setCanWithdraw] = useState<boolean | null>(null);
  const [withdrawn, setWithdrawn] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [variantId, setVariantId] = useState("");
  const [deadline, setDeadline] = useState("");
  const [reason, setReason] = useState("");

  useEffect(() => {
    if (!lessonId) {
      return;
    }
    let cancelled = false;
    setPhase("loading");
    setActionError(null);
    setNotice(null);
    setWithdrawn(false);
    setDraft(null);
    setAssignmentId(null);
    setCanWithdraw(null);
    void loadDraft(lessonId, assignmentFromUrl).then((result) => {
      if (cancelled) {
        return;
      }
      if (result.phase === "ready") {
        setDraft(result.draft);
        setAssignmentId(result.assignmentId);
        setCanWithdraw(result.canWithdraw);
        setPhase("ready");
        return;
      }
      setDraft(null);
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
  }, [lessonId, assignmentFromUrl, reloadKey]);

  async function applyDraft(response: Response) {
    rememberCsrfFromResponse(response);
    if (!response.ok) {
      setActionError(await errorSentence(response));
      return;
    }
    const next = readDraft(await response.json());
    if (next) {
      setDraft(next);
    }
  }

  async function onRemove(id: string) {
    if (!lessonId) {
      return;
    }
    setActionError(null);
    try {
      await applyDraft(
        await sendJson(draftUrl(lessonId), "PATCH", { remove: [id] }),
      );
    } catch {
      setActionError("Нет связи");
    }
  }

  async function onAdd() {
    if (!lessonId || variantId.trim() === "") {
      return;
    }
    setActionError(null);
    try {
      await applyDraft(
        await sendJson(draftUrl(lessonId), "PATCH", {
          add: [variantId.trim()],
        }),
      );
      setVariantId("");
    } catch {
      setActionError("Нет связи");
    }
  }

  async function onDeadline() {
    if (!lessonId) {
      return;
    }
    const utc = minskLocalToUtc(deadline);
    if (!utc) {
      return;
    }
    setActionError(null);
    try {
      await applyDraft(
        await sendJson(draftUrl(lessonId), "PATCH", { deadline: utc }),
      );
    } catch {
      setActionError("Нет связи");
    }
  }

  async function onIssue() {
    if (!lessonId) {
      return;
    }
    setActionError(null);
    try {
      const response = await sendJson(draftUrl(lessonId, "issue"), "POST");
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      const view = readTutorView(await response.json());
      if (!view) {
        return;
      }
      setDraft((current) => (current ? { ...current, issued: true } : current));
      setAssignmentId(view.id);
      setCanWithdraw(view.canWithdraw);
      setNotice("выдано");
    } catch {
      setActionError("Нет связи");
    }
  }

  async function onWithdraw() {
    if (!assignmentId || canWithdraw !== true || withdrawn) {
      return;
    }
    setActionError(null);
    try {
      const response = await sendJson(
        `/assignments/${encodeURIComponent(assignmentId)}/withdraw`,
        "POST",
        { reason },
      );
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        if (response.status === 409) {
          setCanWithdraw(false);
        }
        setActionError(await errorSentence(response));
        return;
      }
      setWithdrawn(true);
      setCanWithdraw(false);
      setNotice("отозвано");
    } catch {
      setActionError("Нет связи");
    }
  }

  const withdrawEnabled =
    draft?.issued === true &&
    !withdrawn &&
    assignmentId !== null &&
    canWithdraw === true;
  const showBar = phase === "ready" && draft !== null;

  return (
    <Screen title="Просмотр ДЗ" withActionBar={showBar}>
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
        {phase === "ready" && draft ? (
          <>
            {draft.warnings.map((warning) => (
              <Text key={warning}>{warning}</Text>
            ))}
            {draft.hints.map((hint) => (
              <Text key={hint}>{hint}</Text>
            ))}
            {draft.deadline ? <Text>{minskTime(draft.deadline)}</Text> : null}
            {draft.items.map((item, index) => (
              <Stack key={`${item.variantId}-${index}`} gap={0}>
                {item.statement ? <Text>{item.statement}</Text> : null}
                <Text>{item.variantId}</Text>
                {!draft.issued ? (
                  <Button
                    type="button"
                    variant="default"
                    onClick={() => void onRemove(item.variantId)}
                  >
                    убрать
                  </Button>
                ) : null}
              </Stack>
            ))}
            {!draft.issued ? (
              <>
                <TextInput
                  aria-label="вариант"
                  classNames={{ input: classes.field }}
                  value={variantId}
                  onChange={(event) => setVariantId(event.currentTarget.value)}
                />
                <Button
                  type="button"
                  variant="default"
                  onClick={() => void onAdd()}
                >
                  добавить
                </Button>
                <TextInput
                  aria-label="срок"
                  classNames={{ input: classes.field }}
                  value={deadline}
                  onChange={(event) => setDeadline(event.currentTarget.value)}
                />
                <Button
                  type="button"
                  variant="default"
                  onClick={() => void onDeadline()}
                >
                  срок
                </Button>
              </>
            ) : null}
            {withdrawEnabled ? (
              <TextInput
                aria-label="причина"
                classNames={{ input: classes.field }}
                value={reason}
                onChange={(event) => setReason(event.currentTarget.value)}
              />
            ) : null}
            {notice ? <Text>{notice}</Text> : null}
            {actionError ? <Text>{actionError}</Text> : null}
          </>
        ) : null}
      </Stack>
      {showBar && draft ? (
        <ActionBar
          primary={
            draft.issued
              ? {
                  label: "Отозвать",
                  onClick: () => void onWithdraw(),
                  disabled: !withdrawEnabled,
                }
              : { label: "Выдать", onClick: () => void onIssue() }
          }
        />
      ) : null}
    </Screen>
  );
}
