import { MantineProvider } from "@mantine/core";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { App } from "../App";
import { SessionsPage } from "./SessionsPage";

// Пакет W8, экран «Сеансы» (web.md: устройства, сеансы, «отозвать все»).
// Маршрут: /tutor/sessions. Ключ реестра — «Репетитор/Сеансы».
// GET /me/sessions — SessionInfo. POST /me/sessions/revoke-all.
// Подтверждение отзыва — «отозвано» (WB-R31). Время — Europe/Minsk (WB-R2).
// ВОПРОС: слова «загрузка» и «пусто» не заданы.
// ВОПРОС: русских подписей state active и pending в web.md нет. Тест их не требует.

const CSRF = "csrf-w8";
const SESSION = "sess-w8-a";
const DEVICE = "телефон-w8";
const KEY = "918273";
const STACK = "goroutine 7 [running]";

type CallRecord = {
  url: string;
  method: string;
  csrf: string | null;
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
});

function sessionsBody() {
  return [
    {
      session_id: SESSION,
      device: DEVICE,
      created_at: "2026-11-19T07:15:00Z",
      expires_at: "2026-11-19T18:40:00Z",
      state: "active",
      current: true,
      key: KEY,
    },
  ];
}

function errorBody(code: string) {
  return { code, message: STACK, details: { key: KEY } };
}

function installFetch(
  reply: (method: string, path: string) => Response | "network",
) {
  sessionStorage.setItem("ai-mentor.csrf", CSRF);
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "http://localhost",
      );
      const method = (
        init?.method ?? (input instanceof Request ? input.method : "GET")
      ).toUpperCase();
      const path = url.pathname.replace(/\/$/, "") || "/";
      const result = reply(method, path);
      if (result === "network") {
        throw new TypeError("Failed to fetch");
      }
      return result;
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderSessions() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={["/tutor/sessions"]}>
        <Routes>
          <Route path="/tutor/sessions" element={<SessionsPage />} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );
}

async function callRecords(
  fetchMock: ReturnType<typeof vi.fn>,
): Promise<CallRecord[]> {
  const records: CallRecord[] = [];
  for (const call of fetchMock.mock.calls) {
    const [input, init] = call as [RequestInfo | URL, RequestInit | undefined];
    const url = input instanceof Request ? input.url : String(input);
    const method = (
      init?.method ?? (input instanceof Request ? input.method : "GET")
    ).toUpperCase();
    records.push({
      url,
      method,
      csrf: new Headers(init?.headers).get("X-CSRF-Token"),
    });
  }
  return records;
}

function pageText() {
  for (const node of document.body.querySelectorAll("style")) {
    node.textContent = "";
  }
  return document.body.textContent ?? "";
}

test("сеансы: устройство, сеанс и время Минска", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === "/me/sessions") {
      return Response.json(sessionsBody());
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderSessions();
  expect(await screen.findByText(DEVICE)).toBeTruthy();
  expect(screen.getByText(SESSION)).toBeTruthy();
  expect(pageText()).toContain("10:15");
  expect(pageText()).toContain("21:40");
  expect(screen.getByRole("heading", { name: "Сеансы" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "отозвать все" })).toBeTruthy();
  expect(pageText()).not.toContain(KEY);
  expect(pageText()).not.toContain("active");
  expect(pageText()).not.toContain(STACK);
});

test("«отозвать все» отзывает сеансы и подтверждает «отозвано»", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === "/me/sessions") {
      return Response.json(sessionsBody());
    }
    if (method === "POST" && path === "/me/sessions/revoke-all") {
      return new Response(null, { status: 204 });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderSessions();
  fireEvent.click(await screen.findByRole("button", { name: "отозвать все" }));
  expect(await screen.findByText("отозвано")).toBeTruthy();
  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    const posted = records.filter((record) => record.method === "POST");
    expect(posted).toHaveLength(1);
    expect(posted[0]?.url).toBe("/me/sessions/revoke-all");
    expect(posted[0]?.csrf).toBe(CSRF);
  });
  expect(pageText()).not.toContain(STACK);
});

test("пустой список остаётся экраном «Сеансы»", async () => {
  installFetch(() => Response.json([]));
  renderSessions();
  expect(
    await screen.findByRole("button", { name: "отозвать все" }),
  ).toBeTruthy();
  expect(screen.getByRole("heading", { name: "Сеансы" })).toBeTruthy();
  expect(screen.queryByText(SESSION)).toBeNull();
});

test("wb-r30: сеансы без сети — «Нет связи» и «Повторить»", async () => {
  let calls = 0;
  installFetch((method, path) => {
    if (method === "GET" && path === "/me/sessions") {
      calls += 1;
      if (calls === 1) {
        return "network";
      }
      return Response.json(sessionsBody());
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderSessions();
  expect(await screen.findByText("Нет связи")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(await screen.findByText(DEVICE)).toBeTruthy();
});

test("wb-r6: ошибка сеансов — текст из таблицы, не стек", async () => {
  installFetch(() => Response.json(errorBody("login_stale"), { status: 401 }));
  renderSessions();
  expect(await screen.findByText("Войдите ещё раз")).toBeTruthy();
  expect(pageText()).not.toContain(STACK);
  expect(pageText()).not.toContain(KEY);
});

test("csrf при отзыве показывает текст ошибки", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === "/me/sessions") {
      return Response.json(sessionsBody());
    }
    if (method === "POST" && path === "/me/sessions/revoke-all") {
      return Response.json(errorBody("csrf"), { status: 403 });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderSessions();
  fireEvent.click(await screen.findByRole("button", { name: "отозвать все" }));
  expect(await screen.findByText("Обновите страницу и повторите")).toBeTruthy();
  expect(screen.queryByText("отозвано")).toBeNull();
});

test("маршрут «Сеансы» открывается из приложения", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === "/me/sessions") {
      return Response.json([]);
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={["/tutor/sessions"]}>
        <App />
      </MemoryRouter>
    </MantineProvider>,
  );
  expect(await screen.findByRole("heading", { name: "Сеансы" })).toBeTruthy();
});
