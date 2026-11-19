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
import { HomeworkResultsPage } from "./HomeworkResultsPage";

// Пакет W7, экран «Итоги ДЗ» (web.md, WB-R24, WB-R30, D138, RC-R20, AT-R18, AT-R19).
// Маршрут: /tutor/students/:studentId/assignments/:assignmentId.
// Ключ реестра — «Репетитор/Итоги ДЗ».
// GET summary, attempts, disputes; POST manual-verdict; GET /rechecks/{correction}.
// До «Сдать» summary отвечает not_submitted: это не ошибка экрана, попытки с пометкой.
// ВОПРОС: подпись вердикта skipped в StatusBadge нет. Тест его не шлёт.
// ВОПРОС: слова «загрузка» и «пусто» не заданы.

const CSRF = "csrf-w7";
const STUDENT = "stu-w7";
const ASSIGNMENT = "asg-w7";
const ITEM = "item-w7-a";
const ATTEMPT = "att-w7";
const RAW = "сырой-13";
const KEY = "918273";
const SOLUTION = "ход-w7-скрыт";
const MARK = "верная опция";
const DISPUTE_TEXT = "условие про другое";
const FOREIGN = "чужой-спор";
const CHANGED = "asg-w7-changed";
const SKIPPED = "att-w7-skip";
const STACK = "goroutine 7 [running]";
const PATH = `/tutor/students/${STUDENT}/assignments/${ASSIGNMENT}`;

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
    assignment_id: ASSIGNMENT,
    items: [
      {
        item_id: ITEM,
        verdict: "correct",
        points: 4,
        max_points: 4,
        show: { key: KEY, solution_block: SOLUTION },
      },
    ],
    key: KEY,
  };
}

function attemptBody(submittedVerdict: string | null = "wrong") {
  return [
    {
      attempt_id: ATTEMPT,
      item_id: ITEM,
      attempt_no: 1,
      raw_input: RAW,
      verdict: submittedVerdict,
      points: 0,
      key: KEY,
      solution_block: SOLUTION,
      note: MARK,
    },
  ];
}

function disputeBody(withReport = false) {
  return [
    {
      id: "dsp-w7",
      assignment_id: ASSIGNMENT,
      item_id: ITEM,
      reason: "key_wrong",
      state: "open",
      text: DISPUTE_TEXT,
      correction_id: withReport ? 7 : undefined,
    },
    {
      id: "dsp-w7-b",
      assignment_id: ASSIGNMENT,
      item_id: "item-w7-b",
      reason: "statement_error",
      state: "open",
      text: "",
    },
    {
      id: "dsp-w7-c",
      assignment_id: ASSIGNMENT,
      item_id: "item-w7-c",
      reason: "equivalent_answer",
      state: "resolved",
      text: "",
    },
    {
      id: "dsp-w7-d",
      assignment_id: ASSIGNMENT,
      item_id: "item-w7-d",
      reason: "other",
      state: "open",
      text: "",
    },
    {
      id: "dsp-foreign",
      assignment_id: "asg-other",
      item_id: "item-other",
      reason: "other",
      text: FOREIGN,
      state: "open",
    },
  ];
}

function reportBody() {
  return {
    correction_id: 7,
    state: "done",
    total: 3,
    changed: 1,
    changed_assignments: [
      { assignment_id: CHANGED, student_id: STUDENT, items_changed: 1 },
    ],
    skipped_manual_attempts: [SKIPPED],
    key: KEY,
  };
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

function renderResults() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={[PATH]}>
        <Routes>
          <Route
            path="/tutor/students/:studentId/assignments/:assignmentId"
            element={<HomeworkResultsPage />}
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
  expect(text).not.toContain(FOREIGN);
  expect(text).not.toContain(STACK);
  expect(text).not.toContain("done");
}

test("итоги: результат, споры и отчёт перепроверки, без ключа", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/summary`) {
      return Response.json(summaryBody());
    }
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/attempts`) {
      return Response.json(attemptBody());
    }
    if (method === "GET" && path === `/students/${STUDENT}/disputes`) {
      return Response.json(disputeBody(true));
    }
    if (method === "GET" && path === "/rechecks/7") {
      return Response.json(reportBody());
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderResults();

  expect(await screen.findByText(RAW)).toBeTruthy();
  expect(screen.getAllByText("верно").length).toBeGreaterThan(0);
  expect(screen.getAllByText("неверно").length).toBeGreaterThan(0);
  expect(screen.getAllByText(ITEM).length).toBeGreaterThan(0);
  expect(screen.getByText("4")).toBeTruthy();
  expect(screen.getByText("ключ неверен")).toBeTruthy();
  expect(screen.getByText("ошибка в условии")).toBeTruthy();
  expect(screen.getByText("мой ответ эквивалентен")).toBeTruthy();
  expect(screen.getByText("другое")).toBeTruthy();
  expect(screen.getByText(DISPUTE_TEXT)).toBeTruthy();
  expect(
    screen.getByRole("heading", { name: "отчёт перепроверки" }),
  ).toBeTruthy();
  expect(screen.getByText(CHANGED)).toBeTruthy();
  expect(screen.getByText(STUDENT)).toBeTruthy();
  expect(screen.getByText(SKIPPED)).toBeTruthy();
  expect(screen.queryByText("ещё не сдано")).toBeNull();
  expect(screen.getByRole("button", { name: "ручной вердикт" })).toBeTruthy();
  expectHidden();
});

test("wb-r24: до сдачи сырые попытки помечены «ещё не сдано»", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/summary`) {
      return Response.json(
        { code: "not_submitted", message: STACK, key: KEY },
        { status: 409 },
      );
    }
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/attempts`) {
      return Response.json(attemptBody(null));
    }
    if (method === "GET" && path === `/students/${STUDENT}/disputes`) {
      return Response.json([]);
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderResults();
  expect(await screen.findByText("ещё не сдано")).toBeTruthy();
  expect(screen.getByText(RAW)).toBeTruthy();
  expect(pageText()).not.toContain("Спор можно открыть после сдачи ДЗ");
  expectHidden();
});

test("ручной вердикт уходит на попытку", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/summary`) {
      return Response.json(summaryBody());
    }
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/attempts`) {
      return Response.json(attemptBody());
    }
    if (method === "GET" && path === `/students/${STUDENT}/disputes`) {
      return Response.json([]);
    }
    if (method === "POST" && path === `/attempts/${ATTEMPT}/manual-verdict`) {
      return Response.json({
        attempt_id: ATTEMPT,
        verdict: "correct",
        points: 2,
        key: KEY,
      });
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderResults();
  expect(await screen.findByText(RAW)).toBeTruthy();
  fireEvent.change(screen.getByRole("combobox", { name: "вердикт" }), {
    target: { value: "partial" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "баллы" }), {
    target: { value: "2" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "причина" }), {
    target: { value: "эквивалентная запись" },
  });
  fireEvent.click(screen.getByRole("button", { name: "ручной вердикт" }));

  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    const posted = records.filter((record) => record.method === "POST");
    expect(posted).toHaveLength(1);
    expect(posted[0]?.url).toBe(`/attempts/${ATTEMPT}/manual-verdict`);
    expect(JSON.parse(posted[0]?.body ?? "")).toEqual({
      verdict: "partial",
      points: 2,
      reason: "эквивалентная запись",
    });
    expect(posted[0]?.csrf).toBe(CSRF);
  });
  expectHidden();
});

test("wb-r30: итоги без сети — «Нет связи» и «Повторить»", async () => {
  let calls = 0;
  installFetch((method, path) => {
    if (method !== "GET") {
      return Response.json(errorBody("forbidden"), { status: 404 });
    }
    calls += 1;
    if (calls <= 3) {
      return "network";
    }
    if (path === `/assignments/${ASSIGNMENT}/summary`) {
      return Response.json({ assignment_id: ASSIGNMENT, items: [] });
    }
    if (path.endsWith("/attempts") || path.endsWith("/disputes")) {
      return Response.json([]);
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  renderResults();
  expect(await screen.findByText("Нет связи")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(
    await screen.findByRole("heading", { name: "результаты по задачам" }),
  ).toBeTruthy();
});

test("wb-r6: ошибка итогов — текст из таблицы, не стек", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/summary`) {
      return Response.json(errorBody("forbidden"), { status: 403 });
    }
    return Response.json([]);
  });
  renderResults();
  expect(
    await screen.findByText("Нет доступа или объекта не существует"),
  ).toBeTruthy();
  expectHidden();
});

test("маршрут «Итоги ДЗ» открывается из приложения", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/assignments/${ASSIGNMENT}/summary`) {
      return Response.json({ assignment_id: ASSIGNMENT, items: [] });
    }
    if (path.endsWith("/attempts") || path.endsWith("/disputes")) {
      return Response.json([]);
    }
    return Response.json(errorBody("forbidden"), { status: 404 });
  });
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[PATH]}>
        <App />
      </MemoryRouter>
    </MantineProvider>,
  );
  expect(await screen.findByRole("heading", { name: "Итоги ДЗ" })).toBeTruthy();
});
