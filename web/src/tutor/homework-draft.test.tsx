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
import { HomeworkDraftPage } from "./HomeworkDraftPage";

// Пакет W7, экран «Просмотр ДЗ» (web.md, WB-R23, WB-R30, WB-R31, D195).
// Маршрут: /tutor/lessons/:lessonId/homework. Ключ реестра — «Репетитор/Просмотр ДЗ».
// GET/PATCH /lessons/{lesson}/homework-draft, POST .../issue, POST /assignments/{id}/withdraw.
// ТРАКТОВКА: id выдачи в HomeworkDraft нет. После «Выдать» он берётся из ответа.
// Повторное открытие передаёт его как ?assignment=; попытки решают, доступна ли «Отозвать».
// ВОПРОС: слова «загрузка» и «пусто» не заданы.
// ВОПРОС: текста для already_issued, deadline_required и lesson_not_marked в web.md нет.

const CSRF = "csrf-w7";
const LESSON = "lesson-w7";
const ASSIGNMENT = "asg-w7";
const STATEMENT = "Найдите выдуманную долю.";
const VARIANT = "var-w7-a";
const ADDED = "var-w7-b";
const KEY = "918273";
const SOLUTION = "ход-w7-скрыт";
const MARK = "верная опция";
const HINT = "на уроке решено 5, в ДЗ 16";
const WARNING = "предупреждение-w7";
const DEADLINE = "2026-11-20T15:00:00Z";
const NEXT_LOCAL = "2026-11-21T21:30";
const NEXT_UTC = "2026-11-21T18:30:00Z";
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

function draftBody(issued = false, variant = VARIANT) {
  return {
    lesson_id: LESSON,
    deadline: DEADLINE,
    items: [
      {
        variant_id: variant,
        type_id: "type-w7",
        statement: STATEMENT,
        key: KEY,
        solution_block: SOLUTION,
        options: [{ n: 1, text: MARK, correct: true }],
      },
    ],
    hints: [HINT],
    warnings: [{ type: "variants_needed", text: WARNING }],
    discuss_topics: ["не-показывать-тему"],
    issued,
  };
}

function errorBody(code: string) {
  return {
    code,
    message: STACK,
    details: { key: KEY, solution_block: SOLUTION },
  };
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

function renderDraft(search = "") {
  return render(
    <MantineProvider>
      <MemoryRouter
        initialEntries={[`/tutor/lessons/${LESSON}/homework${search}`]}
      >
        <Routes>
          <Route
            path="/tutor/lessons/:lessonId/homework"
            element={<HomeworkDraftPage />}
          />
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
    const headers = new Headers(init?.headers);
    records.push({ url, method, body, csrf: headers.get("X-CSRF-Token") });
  }
  return records;
}

function pageText() {
  for (const node of document.body.querySelectorAll("style")) {
    node.textContent = "";
  }
  const values = [...document.querySelectorAll("input, textarea, select")].map(
    (node) => (node as HTMLInputElement).value,
  );
  return `${document.body.textContent ?? ""}\n${values.join("\n")}`;
}

function expectHidden() {
  const text = pageText();
  const html = document.body.innerHTML;
  expect(text).not.toContain(KEY);
  expect(html).not.toContain(KEY);
  expect(text).not.toContain(SOLUTION);
  expect(html).not.toContain(SOLUTION);
  expect(text).not.toContain(MARK);
  expect(html).not.toContain(MARK);
  expect(text).not.toContain(STACK);
  expect(text).not.toContain("не-показывать-тему");
}

test("черновик: вариант, срок, подсказка и «Выдать», без ключа", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/homework-draft`) {
      return Response.json(draftBody());
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderDraft();

  expect(await screen.findByText(STATEMENT)).toBeTruthy();
  expect(screen.getByText(VARIANT)).toBeTruthy();
  expect(screen.getByText(HINT)).toBeTruthy();
  expect(screen.getByText(WARNING)).toBeTruthy();
  expect(pageText()).toContain("18:00");
  expect(screen.getByRole("button", { name: "Выдать" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "убрать" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "добавить" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "срок" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Отозвать" })).toBeNull();
  expect(screen.getByRole("heading", { name: "Просмотр ДЗ" })).toBeTruthy();
  expectHidden();
});

test("убрать и добавить вариант, срок уходит в UTC", async () => {
  const fetchMock = installFetch((method, path) => {
    if (path === `/lessons/${LESSON}/homework-draft`) {
      return Response.json(
        draftBody(false, method === "PATCH" ? ADDED : VARIANT),
      );
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderDraft();
  expect(await screen.findByText(VARIANT)).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "убрать" }));
  fireEvent.change(screen.getByRole("textbox", { name: "вариант" }), {
    target: { value: ADDED },
  });
  fireEvent.click(screen.getByRole("button", { name: "добавить" }));
  fireEvent.change(screen.getByRole("textbox", { name: "срок" }), {
    target: { value: NEXT_LOCAL },
  });
  fireEvent.click(screen.getByRole("button", { name: "срок" }));

  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    const patches = records.filter((record) => record.method === "PATCH");
    expect(patches).toHaveLength(3);
    expect(JSON.parse(patches[0]?.body ?? "")).toEqual({ remove: [VARIANT] });
    expect(JSON.parse(patches[1]?.body ?? "")).toEqual({ add: [ADDED] });
    expect(JSON.parse(patches[2]?.body ?? "")).toEqual({ deadline: NEXT_UTC });
    expect(patches.every((record) => record.csrf === CSRF)).toBe(true);
  });
  expectHidden();
});

test("wb-r23: «Выдать» сменяется на «Отозвать», подтверждение «выдано»", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/homework-draft`) {
      return Response.json(draftBody());
    }
    if (
      method === "POST" &&
      path === `/lessons/${LESSON}/homework-draft/issue`
    ) {
      return Response.json(
        {
          id: ASSIGNMENT,
          kind: "homework",
          state: "issued",
          can_withdraw: true,
          key: KEY,
        },
        { status: 201 },
      );
    }
    if (method === "POST" && path === `/assignments/${ASSIGNMENT}/withdraw`) {
      return new Response(null, { status: 204 });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderDraft();
  expect(await screen.findByRole("button", { name: "Выдать" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Выдать" }));

  expect(await screen.findByText("выдано")).toBeTruthy();
  const withdraw = screen.getByRole("button", { name: "Отозвать" });
  expect(withdraw).toHaveProperty("disabled", false);
  expect(screen.queryByRole("button", { name: "Выдать" })).toBeNull();
  expect(screen.queryByRole("button", { name: "убрать" })).toBeNull();

  fireEvent.change(screen.getByRole("textbox", { name: "причина" }), {
    target: { value: "ошибка сборки" },
  });
  fireEvent.click(withdraw);

  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    const issued = records.filter((record) => record.url.endsWith("/issue"));
    expect(issued).toHaveLength(1);
    expect(issued[0]?.csrf).toBe(CSRF);
    const revoked = records.filter((record) =>
      record.url.endsWith("/withdraw"),
    );
    expect(revoked).toHaveLength(1);
    expect(JSON.parse(revoked[0]?.body ?? "")).toEqual({
      reason: "ошибка сборки",
    });
    expect(revoked[0]?.csrf).toBe(CSRF);
  });
  expect(await screen.findByText("отозвано")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Отозвать" })).toHaveProperty(
    "disabled",
    true,
  );
  expectHidden();
});

test("wb-r23: есть ответы — «Отозвать» недоступна", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/homework-draft`) {
      return Response.json(draftBody());
    }
    if (
      method === "POST" &&
      path === `/lessons/${LESSON}/homework-draft/issue`
    ) {
      return Response.json(
        {
          id: ASSIGNMENT,
          kind: "homework",
          state: "issued",
          can_withdraw: false,
        },
        { status: 201 },
      );
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderDraft();
  fireEvent.click(await screen.findByRole("button", { name: "Выдать" }));
  const withdraw = await screen.findByRole("button", { name: "Отозвать" });
  expect(withdraw).toHaveProperty("disabled", true);
  expectHidden();
});

test("wb-r23: открытый черновик с выдачей без ответов отзывает её", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/homework-draft`) {
      return Response.json(draftBody(true));
    }
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/attempts`) {
      return Response.json([]);
    }
    if (method === "POST" && path === `/assignments/${ASSIGNMENT}/withdraw`) {
      return new Response(null, { status: 204 });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderDraft(`?assignment=${ASSIGNMENT}`);
  const withdraw = await screen.findByRole("button", { name: "Отозвать" });
  expect(withdraw).toHaveProperty("disabled", false);
  expect(screen.queryByRole("button", { name: "Выдать" })).toBeNull();
  fireEvent.click(withdraw);
  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    expect(records.some((record) => record.url.endsWith("/withdraw"))).toBe(
      true,
    );
  });
  expect(await screen.findByText("отозвано")).toBeTruthy();
});

test("wb-r23: открытая выдача с попыткой не отзывается", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/homework-draft`) {
      return Response.json(draftBody(true));
    }
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/attempts`) {
      return Response.json([
        {
          attempt_id: "att-w7",
          item_id: VARIANT,
          attempt_no: 1,
          raw_input: "сырой-черновик",
          verdict: "wrong",
          points: 0,
          key: KEY,
        },
      ]);
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderDraft(`?assignment=${ASSIGNMENT}`);
  const withdraw = await screen.findByRole("button", { name: "Отозвать" });
  expect(withdraw).toHaveProperty("disabled", true);
  expect(pageText()).not.toContain("сырой-черновик");
  expectHidden();
});

test("has_answers: текст ошибки и «Отозвать» недоступна", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/homework-draft`) {
      return Response.json(draftBody());
    }
    if (
      method === "POST" &&
      path === `/lessons/${LESSON}/homework-draft/issue`
    ) {
      return Response.json(
        { id: ASSIGNMENT, state: "issued", can_withdraw: true },
        { status: 201 },
      );
    }
    if (method === "POST" && path === `/assignments/${ASSIGNMENT}/withdraw`) {
      return Response.json(errorBody("has_answers"), { status: 409 });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderDraft();
  fireEvent.click(await screen.findByRole("button", { name: "Выдать" }));
  fireEvent.click(await screen.findByRole("button", { name: "Отозвать" }));
  expect(
    await screen.findByText("Нельзя отозвать: ученик уже отвечал"),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Отозвать" })).toHaveProperty(
    "disabled",
    true,
  );
  expectHidden();
});

test("wb-r30: черновик без сети — «Нет связи» и «Повторить»", async () => {
  let calls = 0;
  installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/homework-draft`) {
      calls += 1;
      if (calls === 1) {
        return "network";
      }
      return Response.json(draftBody());
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderDraft();
  expect(await screen.findByText("Нет связи")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Повторить" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(await screen.findByText(STATEMENT)).toBeTruthy();
});

test("wb-r6: ошибка черновика — текст из таблицы, не стек", async () => {
  installFetch(() => Response.json(errorBody("forbidden"), { status: 403 }));
  renderDraft();
  expect(
    await screen.findByText("Нет доступа или объекта не существует"),
  ).toBeTruthy();
  expectHidden();
});

test("пустой черновик остаётся экраном «Просмотр ДЗ»", async () => {
  installFetch(() =>
    Response.json({
      lesson_id: LESSON,
      items: [],
      hints: [],
      warnings: [],
      issued: false,
    }),
  );
  renderDraft();
  expect(
    await screen.findByRole("heading", { name: "Просмотр ДЗ" }),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Выдать" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "убрать" })).toBeNull();
});

test("маршрут «Просмотр ДЗ» открывается из приложения", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/homework-draft`) {
      return Response.json(draftBody());
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/tutor/lessons/${LESSON}/homework`]}>
        <App />
      </MemoryRouter>
    </MantineProvider>,
  );
  expect(
    await screen.findByRole("heading", { name: "Просмотр ДЗ" }),
  ).toBeTruthy();
});
