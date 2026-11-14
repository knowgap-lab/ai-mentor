import { Button, Stack, Text, Title } from "@mantine/core";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import { List, ListRow, Screen, StatusBadge } from "../ui";
import { rememberCsrfFromResponse } from "../task/csrf";

type Summary = {
  id: string;
  subject: string;
  state: string;
  deadline?: string;
  answered: number;
  total: number;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function readSummary(value: unknown): Summary | null {
  if (!isRecord(value)) {
    return null;
  }
  if (typeof value.id !== "string" || typeof value.subject !== "string") {
    return null;
  }
  if (typeof value.total !== "number") {
    return null;
  }
  const state = typeof value.state === "string" ? value.state : "issued";
  return {
    id: value.id,
    subject: value.subject,
    state,
    deadline: typeof value.deadline === "string" ? value.deadline : undefined,
    answered: typeof value.answered === "number" ? value.answered : 0,
    total: value.total,
  };
}

function formatDeadline(value: string | undefined): string {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return new Intl.DateTimeFormat("ru", {
    timeZone: "Europe/Minsk",
    day: "numeric",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function HomePage() {
  const [rows, setRows] = useState<Summary[]>([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setFailed(false);
      try {
        const response = await fetch("/assignments", {
          credentials: "include",
        });
        rememberCsrfFromResponse(response);
        if (!response.ok) {
          throw new Error("load failed");
        }
        const payload: unknown = await response.json();
        if (!Array.isArray(payload)) {
          throw new Error("load failed");
        }
        const next = payload
          .map((item) => readSummary(item))
          .filter((item): item is Summary => item !== null);
        if (!cancelled) {
          setRows(next);
          setLoading(false);
        }
      } catch {
        if (!cancelled) {
          setRows([]);
          setFailed(true);
          setLoading(false);
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  return (
    <Screen title="ИИ-ментор">
      <Stack gap="md">
        <Button component={Link} to="/next" variant="subtle">
          дальше
        </Button>
        <Title order={2} size="h3">
          Выдачи
        </Title>
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
        {!loading && !failed && rows.length === 0 ? <Text>Пусто</Text> : null}
        {rows.length > 0 ? (
          <List label="Выдачи">
            {rows.map((row) => {
              const deadline = formatDeadline(row.deadline);
              return (
                <ListRow
                  key={row.id}
                  to={`/assignment/${encodeURIComponent(row.id)}`}
                  title={row.subject}
                  subtitle={[
                    deadline,
                    `отвечено ${row.answered} из ${row.total}`,
                  ]
                    .filter((part) => part !== "")
                    .join(" · ")}
                  aside={
                    row.state === "submitted" ? (
                      <StatusBadge state="submitted" />
                    ) : row.state === "withdrawn" ? (
                      <Text size="sm" c="gray.7">
                        отозвано
                      </Text>
                    ) : undefined
                  }
                />
              );
            })}
          </List>
        ) : null}
        {/* В core-api нет списка уроков ученика и поля урока у выдачи. */}
        <section>
          <Title order={2} size="h3">
            Ближайший урок
          </Title>
          <Text>нет</Text>
        </section>
      </Stack>
    </Screen>
  );
}
