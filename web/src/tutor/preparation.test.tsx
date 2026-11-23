import { MantineProvider } from "@mantine/core";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { App } from "../App";
import { PreparationPage } from "./PreparationPage";

// Пакет W6, экран «Подготовка» (web.md, WB-R22, web-15, LS-R24, D38).
// Маршрут теста: /tutor/lessons/:lessonId/preparation.
// Ключ реестра — «Репетитор/Подготовка». web/screens.json этот файл не заполняет.
// GET /lessons/{lesson}/preparation — Preparation. Единственный ответ, где ключ
// приходит до ученика. До кнопки «показать» ключа и решения на экране нет.
// Условие и variant_id видны сразу. Кнопку «показать» тест нажимает, пока такие
// кнопки есть: и одна на экран, и по кнопке на задачу обязаны открыть все ключи.
// Повторный GET при «показать» не нужен: ключ уже в первом ответе.
// ВОПРОС: в Preparation нет вида (урок, ДЗ или срез). Тест заголовки «ДЗ» и «срез» не требует.
// ВОПРОС: одна кнопка «показать» на экран или на задачу. Тест это не различает.
// ВОПРОС: слова «загрузка» и «пусто» не заданы.
// ВОПРОС: как в jsdom проверять ширину 360 (WB-R1). Тест ширину не проверяет.
// «верная опция» этот экран не пишет: в таблице web.md для подготовки её нет.
// Перенос, отмена, пропуск и две отметки — экран «Урок», здесь их нет.

const LESSON = "lesson-w6";
const STATEMENT_A = "Найдите выдуманную долю.";
const STATEMENT_B = "Сколько выдуманных частей.";
const VARIANT_A = "var-w6-a";
const VARIANT_B = "var-w6-b";
const KEY_A = "918273";
const KEY_B = "70415";
const SOLUTION_A = "ход-w6-скрыт";
const SOLUTION_B = "ход-w6-второй";
const OPTION_A = "десять";
const OPTION_B = "два";

const LOAD_ERRORS: [string, number, string][] = [
  ["forbidden", 403, "Нет доступа или объекта не существует"],
  ["unavailable", 503, "Нет связи, попробуйте ещё раз"],
  ["login_stale", 401, "Войдите ещё раз"],
];

type CallRecord = {
  url: string;
  method: string;
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function preparationBody() {
  return {
    lesson_id: LESSON,
    items: [
      {
        variant_id: VARIANT_A,
        statement: STATEMENT_A,
        key: KEY_A,
        solution_block: SOLUTION_A,
        options: [
          { n: 1, text: OPTION_A, correct: false },
          { n: 2, text: OPTION_B, correct: true },
        ],
      },
      {
        variant_id: VARIANT_B,
        statement: STATEMENT_B,
        key: KEY_B,
        solution_block: SOLUTION_B,
      },
    ],
  };
}

function errorBody(code: string) {
  return {
    code,
    message: "goroutine 7 [running]",
    details: { key: KEY_A, solution_block: SOLUTION_A },
  };
}

function installFetch(
  reply: (method: string, path: string) => Response | "network",
) {
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

function happyReply(method: string, path: string): Response | "network" {
  if (method === "GET" && path === `/lessons/${LESSON}/preparation`) {
    return Response.json(preparationBody());
  }
  return Response.json({ code: "forbidden" }, { status: 404 });
}

function renderPreparation() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/tutor/lessons/${LESSON}/preparation`]}>
        <Routes>
          <Route
            path="/tutor/lessons/:lessonId/preparation"
            element={<PreparationPage />}
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
    records.push({ url, method });
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
  expect(text).not.toContain(KEY_A);
  expect(html).not.toContain(KEY_A);
  expect(text).not.toContain(KEY_B);
  expect(html).not.toContain(KEY_B);
  expect(text).not.toContain(SOLUTION_A);
  expect(html).not.toContain(SOLUTION_A);
  expect(text).not.toContain(SOLUTION_B);
  expect(html).not.toContain(SOLUTION_B);
  expect(text).not.toContain("верная");
  expect(text).not.toContain("correct");
}

function revealAll() {
  const seen = new Set<HTMLElement>();
  for (let step = 0; step < 6; step += 1) {
    const buttons = screen.queryAllByRole("button", { name: /^показать$/i });
    const next = buttons.find((button) => !seen.has(button));
    if (!next) {
      return;
    }
    seen.add(next);
    fireEvent.click(next);
  }
}

test("w6: маршрут подготовки в приложении", async () => {
  installFetch(happyReply);
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/tutor/lessons/${LESSON}/preparation`]}>
        <App />
      </MemoryRouter>
    </MantineProvider>,
  );
  expect(
    await screen.findByRole("heading", { name: /^Подготовка$/ }),
  ).toBeTruthy();
  expect(await screen.findByText(STATEMENT_A)).toBeTruthy();
});

test("wb-r22: ключ и решение скрыты, пока не нажато «показать»", async () => {
  const fetchMock = installFetch(happyReply);
  renderPreparation();

  expect(
    await screen.findByRole("heading", { name: /^Подготовка$/ }),
  ).toBeTruthy();
  expect(await screen.findByText(STATEMENT_A)).toBeTruthy();
  expect(screen.getByText(STATEMENT_B)).toBeTruthy();
  expect(screen.getByText(VARIANT_A)).toBeTruthy();
  expect(screen.getByText(VARIANT_B)).toBeTruthy();
  expect(screen.getByText(OPTION_A)).toBeTruthy();
  expect(screen.getByText(OPTION_B)).toBeTruthy();
  expect(
    screen.getAllByRole("button", { name: /^показать$/i }).length,
  ).toBeGreaterThan(0);
  expectHidden();
  expect(screen.queryByRole("button", { name: /^перенос$/i })).toBeNull();
  expect(screen.queryByRole("button", { name: /^отмена$/i })).toBeNull();
  expect(screen.queryByRole("button", { name: /^пропуск$/i })).toBeNull();
  expect(screen.queryByRole("button", { name: /^две отметки$/i })).toBeNull();

  const before = fetchMock.mock.calls.length;
  revealAll();
  expect(fetchMock.mock.calls.length).toBe(before);
  const text = pageText();
  expect(text).toContain(KEY_A);
  expect(text).toContain(KEY_B);
  expect(text).toContain(SOLUTION_A);
  expect(text).toContain(SOLUTION_B);
  expect(text).not.toContain("верная");
  expect(text).not.toContain("goroutine");

  const records = await callRecords(fetchMock);
  expect(
    records.some(
      (record) =>
        record.method === "GET" &&
        record.url.includes(`/lessons/${LESSON}/preparation`),
    ),
  ).toBe(true);
  expect(records.some((record) => record.url.includes("/marks"))).toBe(false);
});

test("w6: пустая подготовка без ключа и без «показать»", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/preparation`) {
      return Response.json({ lesson_id: LESSON, items: [] });
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderPreparation();

  expect(
    await screen.findByRole("heading", { name: /^Подготовка$/ }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: /^показать$/i })).toBeNull();
  expect(screen.queryByText(STATEMENT_A)).toBeNull();
  expectHidden();
});

test("wb-r30: подготовка без сети — «Нет связи» и «Повторить»", async () => {
  let online = false;
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}/preparation`) {
      if (!online) {
        return "network";
      }
      return Response.json(preparationBody());
    }
    return "network";
  });
  renderPreparation();

  expect(await screen.findByText("Нет связи")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Повторить" })).toBeTruthy();
  expect(pageText()).not.toContain("попробуйте ещё раз");
  expect(pageText()).not.toContain("Failed to fetch");
  expect(pageText()).not.toContain("TypeError");
  expectHidden();

  online = true;
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(await screen.findByText(STATEMENT_A)).toBeTruthy();
  expectHidden();
  const gets = (await callRecords(fetchMock)).filter((record) => {
    if (record.method !== "GET") {
      return false;
    }
    const url = new URL(record.url, "http://localhost");
    return url.pathname.replace(/\/$/, "") === `/lessons/${LESSON}/preparation`;
  });
  expect(gets.length).toBeGreaterThanOrEqual(2);
});

test.each(LOAD_ERRORS)(
  "wb-r6: загрузка подготовки, код %s",
  async (code, status, sentence) => {
    installFetch((method, path) => {
      if (method === "GET" && path === `/lessons/${LESSON}/preparation`) {
        return Response.json(errorBody(code), { status });
      }
      return Response.json({ code: "forbidden" }, { status: 404 });
    });
    renderPreparation();

    expect(await screen.findByText(sentence)).toBeTruthy();
    const text = pageText();
    expect(text).not.toContain(code);
    expect(text).not.toContain("goroutine");
    expectHidden();
  },
);
