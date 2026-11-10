import { useEffect, useState } from "react";
import { Link } from "react-router";
import { rememberCsrfFromResponse } from "../task/csrf";
import "../task/task.css";

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

function stateLabel(state: string): string | null {
  if (state === "submitted") {
    return "сдано";
  }
  if (state === "withdrawn") {
    return "отозвано";
  }
  return null;
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
    <main>
      <h1>ИИ-ментор</h1>
      <Link to="/next">дальше</Link>
      <h2>Выдачи</h2>
      {loading ? <p>Загрузка</p> : null}
      {failed ? (
        <>
          <p>Нет связи</p>
          <button
            type="button"
            className="touch-target"
            onClick={() => setReloadKey((value) => value + 1)}
          >
            Повторить
          </button>
        </>
      ) : null}
      {!loading && !failed && rows.length === 0 ? <p>Пусто</p> : null}
      <ul>
        {rows.map((row) => {
          const label = stateLabel(row.state);
          return (
            <li key={row.id}>
              <Link to={`/assignment/${encodeURIComponent(row.id)}`}>
                {row.subject}
              </Link>
              <span>{formatDeadline(row.deadline)}</span>
              <span>{`отвечено ${row.answered} из ${row.total}`}</span>
              {label ? <span>{label}</span> : null}
            </li>
          );
        })}
      </ul>
      {/* В core-api нет списка уроков ученика и поля урока у выдачи. */}
      <section>
        <h2>Ближайший урок</h2>
        <p>нет</p>
      </section>
    </main>
  );
}
