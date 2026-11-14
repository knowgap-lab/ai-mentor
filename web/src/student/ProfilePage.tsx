import { Button, Stack, Text } from "@mantine/core";
import { useEffect, useState } from "react";
import { messageForResponse, OFFLINE_TEXT } from "../api-error";
import { csrfHeaders, rememberCsrfFromResponse } from "../task/csrf";
import { clearOutbox } from "../task/outbox";
import { Screen } from "../ui";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export function ProfilePage() {
  const [name, setName] = useState<string | null>(null);
  const [offline, setOffline] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [revoked, setRevoked] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setName(null);
    setOffline(false);
    setLoadError(null);
    async function load() {
      try {
        const response = await fetch("/me", { credentials: "include" });
        rememberCsrfFromResponse(response);
        if (!response.ok) {
          const text = await messageForResponse(response);
          if (!cancelled) {
            setLoadError(text);
          }
          return;
        }
        const body = (await response.json()) as unknown;
        const display =
          isRecord(body) && typeof body.display_name === "string"
            ? body.display_name
            : null;
        if (!cancelled) {
          setName(display);
          setLoadError(null);
        }
      } catch {
        if (!cancelled) {
          setLoadError(null);
          setOffline(true);
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  async function post(path: "/auth/logout" | "/me/sessions/revoke-all") {
    setActionError(null);
    try {
      const response = await fetch(path, {
        method: "POST",
        credentials: "include",
        headers: csrfHeaders(),
      });
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        setActionError(await messageForResponse(response));
        return;
      }
      if (path === "/auth/logout") {
        clearOutbox();
      }
      if (path === "/me/sessions/revoke-all") {
        setRevoked(true);
      }
    } catch {
      setOffline(true);
    }
  }

  return (
    <Screen title="Профиль">
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
        {name ? <Text>{name}</Text> : null}
        <Button
          type="button"
          variant="default"
          onClick={() => void post("/auth/logout")}
        >
          выход
        </Button>
        <Button
          type="button"
          onClick={() => void post("/me/sessions/revoke-all")}
        >
          выйти на всех устройствах
        </Button>
        {revoked ? <Text>отозвано</Text> : null}
        {actionError ? <Text>{actionError}</Text> : null}
      </Stack>
    </Screen>
  );
}
