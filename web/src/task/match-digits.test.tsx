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
import { TaskPage } from "./TaskPage";

// Ввод ct.match и ct.digits на странице задачи (WB-R12, WB-R13, D136).
// WB-R10, WB-R11 и WB-R14 уже закрыты замороженным TaskPage.test.tsx — тот файл не меняется.
// Ответ ct.match и ct.digits — строка (AnswerRequest в core-api.yaml).
// Список цифр у ct.match — combobox, доступное имя равно букве.
// ВОПРОС: набор букв и список цифр для ct.match. В ItemState такого поля нет.
// Тест берёт три буквы А, Б, В: пример WB-R12 «2 из 3» и строки answer-rules «А1Б3В4».
// Полный набор цифр не фиксируется: выбираются 1, 3 и 4.
// Варианты: всегда А, Б, В; буквы из условия; новое поле контракта.

const CSRF = "csrf-w4";
const HIDDEN = "key-hidden-w4";
const STACK = "goroutine 7 [running]";
const MATCH_STATEMENT = "Сопоставьте выдуманные фигуры.";
const DIGITS_STATEMENT = "Запишите цифры выдуманного набора.";

const ANSWER_ERRORS: [string, number, string][] = [
  ["item_closed", 409, "Эта задача уже закрыта"],
  ["assignment_closed", 409, "ДЗ уже сдано"],
  [
    "idempotency_conflict",
    409,
    "Этот ответ уже отправлялся с другим текстом — обновите страницу",
  ],
  ["forbidden", 403, "Нет доступа или объекта не существует"],
  ["csrf", 403, "Обновите страницу и повторите"],
  ["rate_limited", 429, "Слишком часто, подождите немного"],
  ["unavailable", 503, "Нет связи, попробуйте ещё раз"],
  ["login_stale", 401, "Войдите ещё раз"],
];

type AnswerBody = {
  answer?: unknown;
  idempotency_key?: unknown;
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

function taskAssignment(format: "ct.match" | "ct.digits", statement: string) {
  return {
    id: "asg-w4",
    kind: "homework",
    state: "issued",
    items: [
      {
        item_id: "item-1",
        statement,
        answer_format: format,
        item_state: "open",
        attempts_left: 2,
        key: HIDDEN,
        solution_block: "ct-math:M77.B03",
        options: [{ n: 1, text: "верная опция", correct: true }],
      },
    ],
  };
}

function installFetch(
  format: "ct.match" | "ct.digits",
  post?: { status: number; code: string },
) {
  sessionStorage.setItem("ai-mentor.csrf", CSRF);
  const statement = format === "ct.match" ? MATCH_STATEMENT : DIGITS_STATEMENT;
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
      if (method === "GET" && path === "/assignments/asg-w4") {
        return Response.json(taskAssignment(format, statement));
      }
      if (
        method === "POST" &&
        path === "/assignments/asg-w4/items/item-1/answer"
      ) {
        if (post) {
          return Response.json(
            { code: post.code, message: STACK, details: { key: HIDDEN } },
            { status: post.status },
          );
        }
        return Response.json({ status: "scored", item_state: "open" });
      }
      return Response.json({ code: "forbidden" }, { status: 404 });
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderTask() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={["/task/asg-w4/item-1"]}>
        <Routes>
          <Route path="/task/:assignmentId/:itemId" element={<TaskPage />} />
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

function answerPosts(records: CallRecord[]) {
  return records.filter(
    (record) =>
      record.method === "POST" &&
      record.url.includes("/assignments/asg-w4/items/item-1/answer"),
  );
}

function pageText() {
  const values = [...document.querySelectorAll("input, textarea, select")].map(
    (node) => (node as HTMLInputElement).value,
  );
  return `${document.body.textContent ?? ""}\n${values.join("\n")}`;
}

function unavailable(button: HTMLElement) {
  const el = button as HTMLButtonElement;
  return Boolean(el.disabled) || el.getAttribute("aria-disabled") === "true";
}

function expectNoLeak() {
  const text = pageText();
  expect(text).not.toContain(HIDDEN);
  expect(text).not.toContain("ct-math:M77.B03");
  expect(text.toLowerCase()).not.toContain("верная");
}

function letterList(letter: "А" | "Б" | "В") {
  return screen.getByRole("combobox", {
    name: (value) => value.trim() === letter,
  });
}

test("wb-r17: на задаче нет «не понял» и «оспорить»", async () => {
  installFetch("ct.digits");
  renderTask();
  expect(await screen.findByText(DIGITS_STATEMENT)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "не понял" })).toBeNull();
  expect(screen.queryByRole("button", { name: "оспорить" })).toBeNull();
});

test("wb-r12: две буквы из трёх — «Ответить» недоступна, три буквы уходят строкой", async () => {
  const fetchMock = installFetch("ct.match");
  renderTask();

  expect(await screen.findByText(MATCH_STATEMENT)).toBeTruthy();
  expectNoLeak();
  const answer = screen.getByRole("button", { name: "Ответить" });
  expect(unavailable(answer)).toBe(true);

  fireEvent.change(letterList("А"), { target: { value: "1" } });
  fireEvent.change(letterList("Б"), { target: { value: "3" } });
  expect(unavailable(screen.getByRole("button", { name: "Ответить" }))).toBe(
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));
  expect(answerPosts(await callRecords(fetchMock))).toHaveLength(0);

  fireEvent.change(letterList("В"), { target: { value: "4" } });
  expect(unavailable(screen.getByRole("button", { name: "Ответить" }))).toBe(
    false,
  );
  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));

  await waitFor(async () => {
    const posts = answerPosts(await callRecords(fetchMock));
    expect(posts).toHaveLength(1);
    const body = JSON.parse(posts[0]?.body ?? "") as AnswerBody;
    expect(typeof body.answer).toBe("string");
    const text = String(body.answer);
    const pairs = text.match(/[АБВ][0-9]/g) ?? [];
    expect(text).toBe(pairs.join(""));
    expect(pairs).toHaveLength(3);
    expect(text).toContain("А1");
    expect(text).toContain("Б3");
    expect(text).toContain("В4");
    expect(Object.keys(body).sort()).toEqual(["answer", "idempotency_key"]);
    expect(typeof body.idempotency_key).toBe("string");
    expect(body.idempotency_key).not.toBe("");
    expect(posts[0]?.csrf).toBe(CSRF);
  });
  expectNoLeak();
});

test("wb-r13: повтор «1124» не уходит, верные цифры уходят строкой", async () => {
  const fetchMock = installFetch("ct.digits");
  renderTask();

  const field = await screen.findByRole("textbox");
  expect(await screen.findByText(DIGITS_STATEMENT)).toBeTruthy();
  expect(field.getAttribute("inputmode")).toBe("numeric");
  expectNoLeak();

  fireEvent.change(field, { target: { value: "12a4" } });
  expect((field as HTMLInputElement).value).toMatch(/^\d*$/);
  fireEvent.change(field, { target: { value: "12,5" } });
  expect((field as HTMLInputElement).value).toMatch(/^\d*$/);

  fireEvent.change(field, { target: { value: "1124" } });
  expect(pageText()).toContain("цифры не должны повторяться");
  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));
  expect(answerPosts(await callRecords(fetchMock))).toHaveLength(0);

  fireEvent.change(field, { target: { value: "124" } });
  expect(pageText()).not.toContain("цифры не должны повторяться");
  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));

  await waitFor(async () => {
    const posts = answerPosts(await callRecords(fetchMock));
    expect(posts).toHaveLength(1);
    const body = JSON.parse(posts[0]?.body ?? "") as AnswerBody;
    expect(body.answer).toBe("124");
    expect(Object.keys(body).sort()).toEqual(["answer", "idempotency_key"]);
    expect(typeof body.idempotency_key).toBe("string");
    expect(body.idempotency_key).not.toBe("");
    expect(posts[0]?.csrf).toBe(CSRF);
  });
  expectNoLeak();
});

test.each(ANSWER_ERRORS)(
  "wb-r6: ct.digits, код %s",
  async (code, status, sentence) => {
    const fetchMock = installFetch("ct.digits", { status, code });
    renderTask();

    fireEvent.change(await screen.findByRole("textbox"), {
      target: { value: "124" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Ответить" }));

    await waitFor(async () => {
      expect(answerPosts(await callRecords(fetchMock))).toHaveLength(1);
      expect(pageText()).toContain(sentence);
    });
    const text = pageText();
    expect(text).not.toContain(code);
    expect(text).not.toContain(STACK);
    expect(text).not.toContain(HIDDEN);
  },
);
