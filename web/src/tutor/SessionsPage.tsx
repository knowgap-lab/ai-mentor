import { Button, Stack, Text } from "@mantine/core";
import { useEffect, useState } from "react";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";
import { ActionBar, Screen } from "../ui";
import { errorSentence, isRecord } from "../api-error";
import { minskTime } from "./minsk-time";

type SessionRow = {
  id: string;
  device: string;
  created: string;
  expires: string;
};

type LoadResult =
  | { phase: "offline" }
  | { phase: "error"; sentence: string | null }
  | { phase: "ready"; sessions: SessionRow[] };

function readSession(value: unknown): SessionRow | null {
  if (!isRecord(value) || typeof value.session_id !== "string") {
    return null;
  }
  return {
    id: value.session_id,
    device: typeof value.device === "string" ? value.device : "",
    created:
      typeof value.created_at === "string" ? minskTime(value.created_at) : "",
    expires:
      typeof value.expires_at === "string" ? minskTime(value.expires_at) : "",
  };
}

async function loadSessions(): Promise<LoadResult> {
  try {
    const response = await fetch("/me/sessions", { credentials: "include" });
    rememberCsrfFromResponse(response);
    if (!response.ok) {
      return { phase: "error", sentence: await errorSentence(response) };
    }
    const payload: unknown = await response.json();
    if (!Array.isArray(payload)) {
      return { phase: "error", sentence: null };
    }
    return {
      phase: "ready",
      sessions: payload
        .map((item) => readSession(item))
        .filter((item): item is SessionRow => item !== null),
    };
  } catch {
    return { phase: "offline" };
  }
}

export function SessionsPage() {
  const [reloadKey, setReloadKey] = useState(0);
  const [phase, setPhase] = useState<"loading" | "offline" | "error" | "ready">(
    "loading",
  );
  const [errorText, setErrorText] = useState<string | null>(null);
  const [sessions, setSessions] = useState<SessionRow[] | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [revoked, setRevoked] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setPhase("loading");
    setActionError(null);
    setRevoked(false);
    void loadSessions().then((result) => {
      if (cancelled) {
        return;
      }
      if (result.phase === "ready") {
        setSessions(result.sessions);
        setPhase("ready");
        return;
      }
      setSessions(null);
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
  }, [reloadKey]);

  async function onRevoke() {
    setActionError(null);
    try {
      const response = await fetch("/me/sessions/revoke-all", {
        method: "POST",
        credentials: "include",
        headers: csrfHeaders(),
      });
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      setRevoked(true);
    } catch {
      setActionError("Нет связи");
    }
  }

  const showBar = phase === "ready" && sessions !== null;

  return (
    <Screen title="Сеансы" withActionBar={showBar}>
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
        {phase === "ready" && sessions
          ? sessions.map((session) => (
              <Stack key={session.id} gap={0}>
                {session.device ? <Text>{session.device}</Text> : null}
                <Text>{session.id}</Text>
                {session.created ? <Text>{session.created}</Text> : null}
                {session.expires ? <Text>{session.expires}</Text> : null}
              </Stack>
            ))
          : null}
        {revoked ? <Text>отозвано</Text> : null}
        {actionError ? <Text>{actionError}</Text> : null}
      </Stack>
      {showBar ? (
        <ActionBar
          primary={{
            label: "отозвать все",
            onClick: () => void onRevoke(),
          }}
        />
      ) : null}
    </Screen>
  );
}
