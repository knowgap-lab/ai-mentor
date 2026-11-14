import { MantineProvider } from "@mantine/core";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { SummaryPage } from "./SummaryPage";

// Экран «Итог» (web.md, D104, D232, WB-R17). Маршрут теста:
// /assignment/:assignmentId/summary, параметр assignmentId.
// GET /assignments/{id}/summary — SubmitResult.
// POST /assignments/{id}/items/{item}/did-not-understand
// POST /assignments/{id}/items/{item}/dispute — reason из DisputeRequest.
// Реестр web/screens.json этот файл не заполняет: бриф кладёт его рядом с маршрутом,
// а тесту экрана нужен состав и тексты ошибок.
// ВОПРОС: подпись вердикта skipped. В SubmitResult он есть, в StatusBadge и web.md подписи нет.
// Тест skipped не проверяет.
// ВОПРОС: слова состояний «загрузка» и «пусто» в web.md не заданы. Тест их не фиксирует.
// Сеть — «Нет связи» и «Повторить» (WB-R30).
// Список причин — combobox с пунктами из AT-R18. Если списка ещё нет, тест сначала
// нажимает «оспорить» (форма может быть сразу видна или открываться кнопкой).

const CSRF = "csrf-w4";
const HIDDEN = "key-hidden-w4";
const FAIL_KEY = "84";
const PART_KEY = "73";
const SOLUTION = "ct-math:M77.B03";
const STACK = "goroutine 7 [running]";
const REASONS = [
  "ключ неверен",
  "ошибка в условии",
  "мой ответ эквивалентен",
  "другое",
] as const;

const LOAD_ERRORS: [string, number, string][] = [
  ["forbidden", 403, "Нет доступа или объекта не существует"],
  ["unavailable", 503, "Нет связи, попробуйте ещё раз"],
];

const DISPUTE_ERRORS: [string, number, string][] = [
  ["dispute_exists", 409, "Спор по этой задаче уже открыт"],
  [
    "dispute_text_invalid",
    422,
    "Уберите из текста телефоны, почту и ссылки или сократите до 300 символов",
  ],
  ["not_submitted", 409, "Спор можно открыть после сдачи ДЗ"],
  ["csrf", 403, "Обновите страницу и повторите"],
  ["rate_limited", 429, "Слишком часто, подождите немного"],
];

type DisputeBody = {
  reason?: unknown;
  text?: unknown;
};

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

function summaryBody() {
  return {
    assignment_id: "asg-w4",
    answered: 3,
    skipped: 0,
    items: [
      {
        item_id: "item-ok",
        verdict: "correct",
        points: 1,
        max_points: 1,
        key: HIDDEN,
      },
      {
        item_id: "item-bad",
        verdict: "wrong",
        points: 0,
        max_points: 2,
        show: { key: FAIL_KEY, solution_block: SOLUTION },
      },
      {
        item_id: "item-part",
        verdict: "partial",
        points: 1,
        max_points: 2,
        show: { key: PART_KEY },
      },
    ],
    total_points: 2,
    max_points: 5,
  };
}

function emptySummary() {
  return {
    assignment_id: "asg-w4",
    answered: 0,
    skipped: 0,
    items: [],
    total_points: 0,
    max_points: 0,
  };
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

function renderSummary() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={["/assignment/asg-w4/summary"]}>
        <Routes>
          <Route
            path="/assignment/:assignmentId/summary"
            element={<SummaryPage />}
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

function pageText() {
  const values = [...document.querySelectorAll("input, textarea, select")].map(
    (node) => (node as HTMLInputElement).value,
  );
  return `${document.body.textContent ?? ""}\n${values.join("\n")}`;
}

function words(text: string) {
  return text.split(/[^\p{L}\p{N}]+/u).filter((word) => word !== "");
}

function reasonLists() {
  return screen
    .queryAllByRole("combobox")
    .filter((node) => within(node).queryByRole("option", { name: "другое" }));
}

function disputeButtons() {
  return screen.getAllByRole("button", { name: "оспорить" });
}

function at<T>(items: T[], index: number): T {
  const item = items[index];
  if (item === undefined) {
    throw new Error(`нет элемента ${index}`);
  }
  return item;
}

async function fillDispute(itemIndex: number, text: string | null) {
  await screen.findByRole("heading", { name: "Итог" });
  if (reasonLists().length === 0) {
    fireEvent.click(at(disputeButtons(), itemIndex));
  }
  const lists = reasonLists();
  const fields = screen.getAllByRole("textbox");
  const list = lists[itemIndex] ?? lists[0];
  const field = fields[itemIndex] ?? fields[0];
  if (!list || !field) {
    throw new Error("нет формы спора");
  }
  for (const label of REASONS) {
    expect(within(list).getByRole("option", { name: label })).toBeTruthy();
  }
  const option = within(list).getByRole("option", { name: "другое" });
  fireEvent.change(list, {
    target: { value: (option as HTMLOptionElement).value },
  });
  if (text !== null) {
    fireEvent.change(field, { target: { value: text } });
  }
  return field;
}

test("wb-r17: итог показывает результат, решения неудач и кнопки", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === "/assignments/asg-w4/summary") {
      return Response.json(summaryBody());
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderSummary();

  expect(await screen.findByRole("heading", { name: "Итог" })).toBeTruthy();
  expect(
    await screen.findAllByRole("button", { name: "не понял" }),
  ).toHaveLength(3);
  expect(screen.getAllByRole("button", { name: "оспорить" })).toHaveLength(3);

  const text = pageText();
  const seen = words(text);
  expect(seen).toContain("верно");
  expect(seen).toContain("неверно");
  expect(seen).toContain("частично");
  expect(text).toContain(FAIL_KEY);
  expect(text).toContain(PART_KEY);
  expect(text.includes(SOLUTION) || seen.includes("решение")).toBe(true);
  expect(text).not.toContain(HIDDEN);
});

test("wb-r30: пустой итог без кнопок спора и без ключа", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === "/assignments/asg-w4/summary") {
      return Response.json(emptySummary());
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderSummary();

  expect(await screen.findByRole("heading", { name: "Итог" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "не понял" })).toBeNull();
  expect(screen.queryByRole("button", { name: "оспорить" })).toBeNull();
  expect(pageText()).not.toContain(HIDDEN);
  expect(pageText()).not.toContain(FAIL_KEY);
});

test("wb-r30: итог без сети — «Нет связи» и «Повторить»", async () => {
  let online = false;
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === "/assignments/asg-w4/summary") {
      if (!online) {
        return "network";
      }
      return Response.json(summaryBody());
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderSummary();

  expect(await screen.findByText("Нет связи")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Повторить" })).toBeTruthy();
  expect(pageText()).not.toContain("попробуйте ещё раз");
  expect(pageText()).not.toContain("Failed to fetch");
  expect(pageText()).not.toContain("TypeError");
  expect(pageText()).not.toContain(FAIL_KEY);

  online = true;
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(await screen.findByText(FAIL_KEY)).toBeTruthy();
  const gets = (await callRecords(fetchMock)).filter(
    (record) =>
      record.method === "GET" &&
      record.url.includes("/assignments/asg-w4/summary"),
  );
  expect(gets.length).toBeGreaterThanOrEqual(2);
});

test.each(LOAD_ERRORS)(
  "wb-r6: загрузка итога, код %s",
  async (code, status, sentence) => {
    installFetch((method, path) => {
      if (method === "GET" && path === "/assignments/asg-w4/summary") {
        return Response.json(errorBody(code), { status });
      }
      return Response.json({ code: "forbidden" }, { status: 404 });
    });
    renderSummary();

    expect(await screen.findByText(sentence)).toBeTruthy();
    const text = pageText();
    expect(text).not.toContain(code);
    expect(text).not.toContain(STACK);
    expect(text).not.toContain(HIDDEN);
  },
);

test("wb-r17: «не понял» отправляет запрос по задаче", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === "/assignments/asg-w4/summary") {
      return Response.json(summaryBody());
    }
    if (
      method === "POST" &&
      path === "/assignments/asg-w4/items/item-bad/did-not-understand"
    ) {
      return new Response(null, { status: 204 });
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderSummary();

  const buttons = await screen.findAllByRole("button", { name: "не понял" });
  fireEvent.click(at(buttons, 1));

  await waitFor(async () => {
    const posts = (await callRecords(fetchMock)).filter(
      (record) =>
        record.method === "POST" &&
        record.url.includes("/items/item-bad/did-not-understand"),
    );
    expect(posts).toHaveLength(1);
    expect(posts[0]?.csrf).toBe(CSRF);
    expect(posts[0]?.body === "" || posts[0]?.body === "{}").toBe(true);
  });
  expect(pageText()).not.toContain(HIDDEN);
});

test("wb-r6: «не понял» при недоступности — текст из таблицы", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === "/assignments/asg-w4/summary") {
      return Response.json(summaryBody());
    }
    if (method === "POST" && path.includes("/did-not-understand")) {
      return Response.json(errorBody("unavailable"), { status: 503 });
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderSummary();

  const buttons = await screen.findAllByRole("button", { name: "не понял" });
  fireEvent.click(at(buttons, 1));

  expect(await screen.findByText("Нет связи, попробуйте ещё раз")).toBeTruthy();
  const text = pageText();
  expect(text).not.toContain("unavailable");
  expect(text).not.toContain(STACK);
  expect(text).not.toContain(HIDDEN);
});

test("wb-r17: спор «другое» не длиннее 300 и уходит причиной other", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === "/assignments/asg-w4/summary") {
      return Response.json(summaryBody());
    }
    if (method === "POST" && path.includes("/dispute")) {
      return Response.json({ id: "dsp-w4", state: "open" }, { status: 201 });
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderSummary();

  await screen.findAllByRole("button", { name: "оспорить" });
  const long = "ы".repeat(400);
  const field = await fillDispute(1, long);
  expect((field as HTMLTextAreaElement).value).toBe(long.slice(0, 300));
  fireEvent.click(at(disputeButtons(), 1));

  await waitFor(async () => {
    const posts = (await callRecords(fetchMock)).filter(
      (record) =>
        record.method === "POST" &&
        record.url.includes("/assignments/asg-w4/items/item-bad/dispute"),
    );
    expect(posts).toHaveLength(1);
    const body = JSON.parse(posts[0]?.body ?? "") as DisputeBody;
    expect(body.reason).toBe("other");
    expect(body.text).toBe(long.slice(0, 300));
    expect(Object.keys(body).sort()).toEqual(["reason", "text"]);
    expect(posts[0]?.csrf).toBe(CSRF);
  });
  expect(pageText()).not.toContain(HIDDEN);
});

test.each(DISPUTE_ERRORS)(
  "wb-r6: спор, код %s",
  async (code, status, sentence) => {
    installFetch((method, path) => {
      if (method === "GET" && path === "/assignments/asg-w4/summary") {
        return Response.json(summaryBody());
      }
      if (method === "POST" && path.includes("/dispute")) {
        return Response.json(errorBody(code), { status });
      }
      return Response.json({ code: "forbidden" }, { status: 404 });
    });
    renderSummary();

    await screen.findAllByRole("button", { name: "оспорить" });
    await fillDispute(1, "условия не сходятся");
    fireEvent.click(at(disputeButtons(), 1));

    expect(await screen.findByText(sentence)).toBeTruthy();
    const text = pageText();
    expect(text).not.toContain(code);
    expect(text).not.toContain(STACK);
    expect(text).not.toContain(HIDDEN);
  },
);
