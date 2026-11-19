import { Button, Stack, Text, TextInput } from "@mantine/core";
import { useState } from "react";
import {
  csrfHeaders,
  hasCsrfToken,
  rememberCsrfFromResponse,
} from "../task/csrf";
import { ActionBar, Screen } from "../ui";
import { errorSentence, isRecord } from "./api-error";
import classes from "./field-focus.module.css";
import { isUnderEighteen, minskTime } from "./minsk-time";

// Состав экрана — имя, дата рождения, ссылка согласия и приглашение.
// StudentCreate требует subjects; в составе предметов нет, уходит пустой список.

type Created = {
  id: string;
  name: string;
  birthDate: string;
};

type LinkView = {
  url: string;
  time: string;
};

function readCreated(value: unknown, birthDate: string): Created | null {
  if (!isRecord(value) || typeof value.id !== "string") {
    return null;
  }
  if (typeof value.display_name !== "string") {
    return null;
  }
  return {
    id: value.id,
    name: value.display_name,
    birthDate:
      typeof value.birth_date === "string" ? value.birth_date : birthDate,
  };
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

async function postJson(path: string, body?: unknown): Promise<Response> {
  if (!hasCsrfToken()) {
    const primed = await fetch("/me", { credentials: "include" });
    rememberCsrfFromResponse(primed);
  }
  return fetch(path, {
    method: "POST",
    credentials: "include",
    headers: {
      ...csrfHeaders(),
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export function NewStudentPage() {
  const [name, setName] = useState("");
  const [birthDate, setBirthDate] = useState("");
  const [created, setCreated] = useState<Created | null>(null);
  const [offline, setOffline] = useState(false);
  const [retry, setRetry] = useState<"create" | "consent" | "invite">("create");
  const [actionError, setActionError] = useState<string | null>(null);
  const [consentLink, setConsentLink] = useState<LinkView | null>(null);
  const [invite, setInvite] = useState<LinkView | null>(null);

  async function onCreate() {
    const displayName = name.trim();
    const birth = birthDate.trim();
    if (!/^\d{4}-\d{2}-\d{2}$/.test(birth) || displayName === "") {
      return;
    }
    setActionError(null);
    setOffline(false);
    try {
      const response = await postJson("/students", {
        display_name: displayName,
        birth_date: birth,
        subjects: [],
      });
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      const student = readCreated(await response.json(), birth);
      if (!student) {
        return;
      }
      setCreated(student);
    } catch {
      setRetry("create");
      setOffline(true);
    }
  }

  async function onConsent() {
    if (!created) {
      return;
    }
    setActionError(null);
    setOffline(false);
    const kind = isUnderEighteen(created.birthDate) ? "parent" : "student";
    try {
      const response = await postJson(
        `/students/${encodeURIComponent(created.id)}/consent-links`,
        { kind },
      );
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        setConsentLink(null);
        setActionError(await errorSentence(response));
        return;
      }
      setConsentLink(readLink(await response.json()));
    } catch {
      setRetry("consent");
      setOffline(true);
    }
  }

  async function onInvite() {
    if (!created) {
      return;
    }
    setActionError(null);
    setOffline(false);
    try {
      const response = await postJson(
        `/students/${encodeURIComponent(created.id)}/invites`,
      );
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        setInvite(null);
        setActionError(await errorSentence(response));
        return;
      }
      setInvite(readLink(await response.json()));
    } catch {
      setRetry("invite");
      setOffline(true);
    }
  }

  function onRetry() {
    if (retry === "consent") {
      void onConsent();
      return;
    }
    if (retry === "invite") {
      void onInvite();
      return;
    }
    void onCreate();
  }

  return (
    <Screen title="Новый ученик" withActionBar={created === null}>
      <Stack gap="md" style={{ minWidth: 0, overflowWrap: "anywhere" }}>
        {offline ? (
          <Stack gap="sm">
            <Text>Нет связи</Text>
            <Button type="button" variant="default" onClick={onRetry}>
              Повторить
            </Button>
          </Stack>
        ) : null}
        {created ? <Text>{created.name}</Text> : null}
        {created === null ? (
          <>
            <TextInput
              aria-label="имя"
              classNames={{ input: classes.field }}
              value={name}
              onChange={(event) => setName(event.currentTarget.value)}
            />
            <TextInput
              aria-label="дата рождения"
              classNames={{ input: classes.field }}
              value={birthDate}
              onChange={(event) => setBirthDate(event.currentTarget.value)}
            />
          </>
        ) : (
          <>
            <Button
              type="button"
              variant="default"
              onClick={() => void onConsent()}
            >
              Ссылка согласия
            </Button>
            <Button
              type="button"
              variant="default"
              onClick={() => void onInvite()}
            >
              Приглашение
            </Button>
          </>
        )}
        {actionError ? <Text>{actionError}</Text> : null}
        {consentLink ? <Text>{consentLink.url}</Text> : null}
        {consentLink?.time ? <Text>{consentLink.time}</Text> : null}
        {invite ? <Text>{invite.url}</Text> : null}
        {invite?.time ? <Text>{invite.time}</Text> : null}
      </Stack>
      {created === null ? (
        <ActionBar
          primary={{ label: "Создать", onClick: () => void onCreate() }}
        />
      ) : null}
    </Screen>
  );
}
