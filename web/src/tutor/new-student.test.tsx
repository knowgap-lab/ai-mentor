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
import { NewStudentPage } from "./NewStudentPage";

// Пакет W8, экран «Новый ученик» (web.md: имя, дата рождения, ссылка согласия, приглашение).
// Маршрут: /tutor/students/new. Ключ реестра — «Репетитор/Новый ученик».
// POST /students — StudentCreate. POST consent-links и invites — как в карточке (PV-R12).
// ТРАКТОВКА: слова кнопки создания в web.md нет. Кнопка называет действие «Создать».
// ТРАКТОВКА: предметы в составе экрана не названы; контракт требует subjects, уходит [].
// ВОПРОС: слова «загрузка» и «пусто» не заданы.

const CSRF = "csrf-w8";
const NAME = "Заяц";
const ADULT = "Ёж";
const BIRTH_MINOR = "2010-04-02";
const BIRTH_ADULT = "2000-01-15";
const STUDENT = "stu-w8";
const CONSENT_URL = "https://mentor.example/consent/w8";
const INVITE_URL = "https://mentor.example/invite/w8";
const KEY = "918273";
const STACK = "goroutine 7 [running]";

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

function studentBody(name: string, birth: string) {
  return {
    id: STUDENT,
    display_name: name,
    birth_date: birth,
    status: "active",
    consent: "none",
    subjects: [],
    key: KEY,
  };
}

function linkBody(url: string) {
  return { url, expires_at: "2026-12-03T15:00:00Z", key: KEY };
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

function renderNew() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={["/tutor/students/new"]}>
        <Routes>
          <Route path="/tutor/students/new" element={<NewStudentPage />} />
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
    }
    records.push({
      url,
      method,
      body,
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

function fill(name: string, birth: string) {
  fireEvent.change(screen.getByRole("textbox", { name: "имя" }), {
    target: { value: name },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "дата рождения" }), {
    target: { value: birth },
  });
}

test("новый ученик: имя, дата рождения, затем ссылки", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === "/students") {
      return Response.json(studentBody(NAME, BIRTH_MINOR), { status: 201 });
    }
    if (method === "POST" && path === `/students/${STUDENT}/consent-links`) {
      return Response.json(linkBody(CONSENT_URL), { status: 201 });
    }
    if (method === "POST" && path === `/students/${STUDENT}/invites`) {
      return Response.json(linkBody(INVITE_URL), { status: 201 });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderNew();

  expect(screen.getByRole("heading", { name: "Новый ученик" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Ссылка согласия" })).toBeNull();
  fill(NAME, BIRTH_MINOR);
  fireEvent.click(screen.getByRole("button", { name: "Создать" }));

  expect(
    await screen.findByRole("button", { name: "Ссылка согласия" }),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Приглашение" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Ссылка согласия" }));
  expect(await screen.findByText(CONSENT_URL)).toBeTruthy();
  expect(pageText()).toContain("18:00");
  fireEvent.click(screen.getByRole("button", { name: "Приглашение" }));
  expect(await screen.findByText(INVITE_URL)).toBeTruthy();

  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    const created = records.find((record) => record.url === "/students");
    expect(JSON.parse(created?.body ?? "")).toEqual({
      display_name: NAME,
      birth_date: BIRTH_MINOR,
      subjects: [],
    });
    expect(created?.csrf).toBe(CSRF);
    const consent = records.find((record) =>
      record.url.endsWith("/consent-links"),
    );
    expect(JSON.parse(consent?.body ?? "")).toEqual({ kind: "parent" });
    expect(consent?.csrf).toBe(CSRF);
    const invite = records.find((record) => record.url.endsWith("/invites"));
    expect(invite?.body ?? "").toBe("");
    expect(invite?.csrf).toBe(CSRF);
  });
  expect(pageText()).not.toContain(KEY);
  expect(pageText()).not.toContain(STACK);
});

test("совершеннолетнему ссылка согласия вида student", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === "/students") {
      return Response.json(studentBody(ADULT, BIRTH_ADULT), { status: 201 });
    }
    if (method === "POST" && path === `/students/${STUDENT}/consent-links`) {
      return Response.json(linkBody(CONSENT_URL), { status: 201 });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderNew();
  fill(ADULT, BIRTH_ADULT);
  fireEvent.click(screen.getByRole("button", { name: "Создать" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "Ссылка согласия" }),
  );
  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    const consent = records.find((record) =>
      record.url.endsWith("/consent-links"),
    );
    expect(JSON.parse(consent?.body ?? "")).toEqual({ kind: "student" });
  });
});

test("приглашение без согласия показывает текст ошибки", async () => {
  installFetch((method, path) => {
    if (method === "POST" && path === "/students") {
      return Response.json(studentBody(NAME, BIRTH_MINOR), { status: 201 });
    }
    if (method === "POST" && path === `/students/${STUDENT}/invites`) {
      return Response.json(errorBody("consent_missing"), { status: 423 });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderNew();
  fill(NAME, BIRTH_MINOR);
  fireEvent.click(screen.getByRole("button", { name: "Создать" }));
  fireEvent.click(await screen.findByRole("button", { name: "Приглашение" }));
  expect(
    await screen.findByText(
      "Нужно согласие родителя — обратитесь к репетитору",
    ),
  ).toBeTruthy();
  expect(pageText()).not.toContain(STACK);
  expect(pageText()).not.toContain(KEY);
});

test("wb-r30: создание без сети — «Нет связи» и «Повторить»", async () => {
  let calls = 0;
  installFetch((method, path) => {
    if (method === "POST" && path === "/students") {
      calls += 1;
      if (calls === 1) {
        return "network";
      }
      return Response.json(studentBody(NAME, BIRTH_MINOR), { status: 201 });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderNew();
  fill(NAME, BIRTH_MINOR);
  fireEvent.click(screen.getByRole("button", { name: "Создать" }));
  expect(await screen.findByText("Нет связи")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(
    await screen.findByRole("button", { name: "Ссылка согласия" }),
  ).toBeTruthy();
});

test("маршрут «Новый ученик» открывается из приложения", async () => {
  installFetch(() => Response.json(errorBody("forbidden"), { status: 404 }));
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={["/tutor/students/new"]}>
        <App />
      </MemoryRouter>
    </MantineProvider>,
  );
  expect(screen.getByRole("heading", { name: "Новый ученик" })).toBeTruthy();
  expect(screen.queryByRole("heading", { name: "Ученик" })).toBeNull();
});
