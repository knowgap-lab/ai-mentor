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
import { ProfilePage } from "./ProfilePage";

// Экран «Профиль» (web.md): имя, выход, «выйти на всех устройствах».
// Маршрут теста: /profile.
// GET /me — Me.display_name. POST /auth/logout — выход.
// POST /me/sessions/revoke-all — «выйти на всех устройствах».
// Реестр web/screens.json этот файл не заполняет.
// ВОПРОС: кнопка «выйти на всех устройствах» и контракт revokeAllSessions
// («все сеансы, кроме текущего») читаются по-разному.
// Тест требует POST /me/sessions/revoke-all и слово «отозвано» (WB-R31).
// Одновременный POST /auth/logout не требует.
// Варианты: только revoke-all; revoke-all и logout.
// ВОПРОС: в глоссарии «профиль» — описание программы для составителя.
// Экран ученика в web.md называется «Профиль». Тест берёт название из web.md.
// ВОПРОС: какой текст, если в Me нет display_name. Тест эту фразу не фиксирует.
// ВОПРОС: слова «загрузка» и «пусто» в web.md не заданы. Тест их не фиксирует.

const CSRF = "csrf-w4";
const NAME = "Кедр";
const HIDDEN = "key-hidden-w4";
const STACK = "goroutine 7 [running]";

const LOAD_ERRORS: [string, number, string][] = [
  ["forbidden", 403, "Нет доступа или объекта не существует"],
  ["unavailable", 503, "Нет связи, попробуйте ещё раз"],
  ["login_stale", 401, "Войдите ещё раз"],
];

type CallRecord = {
  url: string;
  method: string;
  body: string;
  csrf: string | null;
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
});

function meBody() {
  return { user_id: "stu-w4", role: "student", display_name: NAME };
}

function errorBody(code: string) {
  return { code, message: STACK, details: { key: HIDDEN } };
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

function renderProfile() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={["/profile"]}>
        <Routes>
          <Route path="/profile" element={<ProfilePage />} />
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
    let body = "";
    if (typeof init?.body === "string") {
      body = init.body;
    } else if (input instanceof Request) {
      body = await input.clone().text();
    }
    const headers = new Headers(init?.headers);
    if (input instanceof Request) {
      input.headers.forEach((value, key) => {
        if (!headers.has(key)) {
          headers.set(key, value);
        }
      });
    }
    records.push({ url, method, body, csrf: headers.get("X-CSRF-Token") });
  }
  return records;
}

function posts(records: CallRecord[], path: string) {
  return records.filter(
    (record) => record.method === "POST" && record.url.includes(path),
  );
}

function pageText() {
  return document.body.textContent ?? "";
}

test("профиль: имя, выход и «выйти на всех устройствах»", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === "/me") {
      return Response.json(meBody());
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderProfile();

  expect(await screen.findByRole("heading", { name: "Профиль" })).toBeTruthy();
  expect(await screen.findByText(NAME)).toBeTruthy();
  expect(screen.getByRole("button", { name: /^выход$/i })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "выйти на всех устройствах" }),
  ).toBeTruthy();
  expect(pageText()).not.toContain(HIDDEN);
});

test("профиль: «выход» отзывает текущий сеанс", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === "/me") {
      return Response.json(meBody());
    }
    if (method === "POST" && path === "/auth/logout") {
      return new Response(null, { status: 204 });
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderProfile();

  expect(await screen.findByText(NAME)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /^выход$/i }));

  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    const logout = posts(records, "/auth/logout");
    expect(logout).toHaveLength(1);
    expect(logout[0]?.csrf).toBe(CSRF);
    expect(posts(records, "/me/sessions/revoke-all")).toHaveLength(0);
  });
  expect(pageText()).not.toContain(STACK);
});

test("профиль: «выйти на всех устройствах» отзывает сеансы", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === "/me") {
      return Response.json(meBody());
    }
    if (method === "POST" && path === "/me/sessions/revoke-all") {
      return new Response(null, { status: 204 });
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderProfile();

  expect(await screen.findByText(NAME)).toBeTruthy();
  fireEvent.click(
    screen.getByRole("button", { name: "выйти на всех устройствах" }),
  );

  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    const revoked = posts(records, "/me/sessions/revoke-all");
    expect(revoked).toHaveLength(1);
    expect(revoked[0]?.csrf).toBe(CSRF);
    expect(pageText()).toContain("отозвано");
  });
  expect(pageText()).not.toContain(HIDDEN);
});

test("wb-r30: профиль без сети — «Нет связи» и «Повторить»", async () => {
  let online = false;
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === "/me") {
      if (!online) {
        return "network";
      }
      return Response.json(meBody());
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderProfile();

  expect(await screen.findByText("Нет связи")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Повторить" })).toBeTruthy();
  expect(pageText()).not.toContain("попробуйте ещё раз");
  expect(pageText()).not.toContain("Failed to fetch");
  expect(pageText()).not.toContain(NAME);

  online = true;
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(await screen.findByText(NAME)).toBeTruthy();
  const gets = (await callRecords(fetchMock)).filter(
    (record) => record.method === "GET" && record.url.includes("/me"),
  );
  expect(gets.length).toBeGreaterThanOrEqual(2);
});

test.each(LOAD_ERRORS)(
  "wb-r6: профиль, код %s",
  async (code, status, sentence) => {
    installFetch((method, path) => {
      if (method === "GET" && path === "/me") {
        return Response.json(errorBody(code), { status });
      }
      return Response.json({ code: "forbidden" }, { status: 404 });
    });
    renderProfile();

    expect(await screen.findByText(sentence)).toBeTruthy();
    const text = pageText();
    expect(text).not.toContain(code);
    expect(text).not.toContain(STACK);
    expect(text).not.toContain(HIDDEN);
    expect(text).not.toContain(NAME);
  },
);

test("wb-r6: выход, код csrf", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === "/me") {
      return Response.json(meBody());
    }
    if (method === "POST" && path === "/auth/logout") {
      return Response.json(errorBody("csrf"), { status: 403 });
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderProfile();

  expect(await screen.findByText(NAME)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /^выход$/i }));

  expect(await screen.findByText("Обновите страницу и повторите")).toBeTruthy();
  const text = pageText();
  expect(text).not.toContain("csrf");
  expect(text).not.toContain(STACK);
  expect(text).not.toContain(HIDDEN);
});

test("wb-r6: выйти на всех устройствах, код unavailable", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === "/me") {
      return Response.json(meBody());
    }
    if (method === "POST" && path === "/me/sessions/revoke-all") {
      return Response.json(errorBody("unavailable"), { status: 503 });
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderProfile();

  expect(await screen.findByText(NAME)).toBeTruthy();
  fireEvent.click(
    screen.getByRole("button", { name: "выйти на всех устройствах" }),
  );

  expect(await screen.findByText("Нет связи, попробуйте ещё раз")).toBeTruthy();
  const text = pageText();
  expect(text).not.toContain("unavailable");
  expect(text).not.toContain(STACK);
  expect(text).not.toContain(HIDDEN);
});
