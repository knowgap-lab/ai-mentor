import { MantineProvider } from "@mantine/core";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { App } from "../App";

// Кабинет ученика. Подмена fetch, сервер не нужен. Выдуманные id.
// GET /assignments/asg-demo — AssignmentStudentView.
// POST /assignments/asg-demo/submit — без confirm_unanswered отвечает 409
// {code: unanswered_confirmation_required, details.left} (AT-R13).
// GET /lessons/lesson-demo/conspect — Conspect.
// Кнопка «Скачать PDF» запрашивает GET /lessons/lesson-demo/conspect.pdf.
// Ответ части В и очередь — уже существующий маршрут /task/:assignmentId/:itemId,
// POST /assignments/{id}/items/{item}/answer.
// «Связь вернулась» в wb-r18: navigator.onLine и событие window online.
// ВОПРОС: откуда на главной «ближайший урок»; в AssignmentSummary поля урока нет,
// списка уроков ученика в core-api.yaml нет. Тест маршрут / и урок на главной не проверяет.
// ВОПРОС: чем состояние «закрыто» отличается от «верно», когда item_state уже final.
// Тест «закрыто» не проверяет.
// ВОПРОС: какая подпись у второго шага «Сдать» после «осталось N»; в web.md её нет.
// Тест confirm_unanswered не нажимает.
// ВОПРОС: какой текст показать на код pdf_unavailable; в таблице раздела 5 web.md его нет.
// Тест фразу ошибки PDF не фиксирует.

const KEY = "91";
const SOLUTION = "ct-math:M99.B00";
const MARK = "верная опция";
const STATEMENT = "Найдите выдуманное значение.";
const CLEAN_BLOCK = "Сложение выдуманных чисел.";
const SCRIPT_PAYLOAD = "xss-w3";
const STACK = "goroutine 7 [running]: runtime.stack stack.go:41";

const TASKS = [
  "Условие альфа",
  "Условие бета",
  "Условие гамма",
  "Условие дельта",
];

type AnswerBody = {
  answer?: unknown;
  idempotency_key?: unknown;
};

type CallRecord = {
  url: string;
  method: string;
  body: string;
};

type FetchOpts = {
  assignment?: unknown;
  conspect?: unknown;
  answer?: unknown;
  online?: () => boolean;
  sentBodies?: string[];
  summaryTotal?: number;
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  setOnline(true);
});

function setOnline(value: boolean) {
  Object.defineProperty(window.navigator, "onLine", {
    configurable: true,
    get() {
      return value;
    },
  });
}

function asUrl(input: RequestInfo | URL): URL {
  const raw = input instanceof Request ? input.url : String(input);
  return new URL(raw, "http://localhost");
}

function asMethod(input: RequestInfo | URL, init?: RequestInit): string {
  const fromInit = init?.method;
  const fromRequest = input instanceof Request ? input.method : "GET";
  return (fromInit ?? fromRequest).toUpperCase();
}

function pathOf(input: RequestInfo | URL): string {
  return asUrl(input).pathname.replace(/\/$/, "") || "/";
}

async function readBody(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<string> {
  if (typeof init?.body === "string") {
    return init.body;
  }
  if (input instanceof Request) {
    return input.clone().text();
  }
  return "";
}

function openChoiceAssignment() {
  return {
    id: "asg-demo",
    kind: "homework",
    subject: "математика",
    state: "issued",
    deadline: "2026-12-01T18:00:00Z",
    items: [
      {
        item_id: "item-1",
        statement: STATEMENT,
        answer_format: "ct.choice1",
        item_state: "open",
        attempts_left: 1,
        key: KEY,
        solution_block: SOLUTION,
        show: { key: KEY, solution_block: SOLUTION },
        options: [
          { n: 1, text: "север", correct: true, mark: MARK },
          { n: 2, text: "юг", correct: false },
          { n: 3, text: "восток" },
          { n: 4, text: "запад" },
          { n: 5, text: "зенит" },
        ],
      },
    ],
  };
}

function integerAssignment() {
  return {
    id: "asg-demo",
    kind: "homework",
    subject: "математика",
    state: "issued",
    deadline: "2026-12-01T18:00:00Z",
    items: [
      {
        item_id: "item-1",
        statement: STATEMENT,
        answer_format: "ct.integer",
        item_state: "open",
        attempts_left: 2,
        key: KEY,
        solution_block: SOLUTION,
        show: { key: KEY },
      },
    ],
  };
}

function fourOpenAssignment() {
  return {
    id: "asg-demo",
    kind: "homework",
    subject: "математика",
    state: "issued",
    deadline: "2026-12-01T18:00:00Z",
    items: TASKS.map((statement, index) => ({
      item_id: `item-${index + 1}`,
      statement,
      answer_format: "ct.integer",
      item_state: "open",
      attempts_left: 2,
      key: KEY,
      solution_block: SOLUTION,
    })),
  };
}

function conspect(blocks: { id: string; markdown: string }[]) {
  return {
    lesson_id: "lesson-demo",
    catalog_version: 3,
    blocks: blocks.map((block) => ({
      id: block.id,
      kind: "theory",
      markdown: block.markdown,
    })),
  };
}

function lessonCard() {
  return {
    id: "lesson-demo",
    template_id: "tpl-demo",
    place_no: 1,
    title: "Выдуманная тема",
    state: "held",
  };
}

function installFetch(opts: FetchOpts) {
  const sentBodies = opts.sentBodies ?? [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = asMethod(input, init);
      const path = pathOf(input);
      if (method === "GET" && path === "/me") {
        return Response.json({
          user_id: "student-demo",
          role: "student",
          display_name: "Рысь",
        });
      }
      if (method === "GET" && path === "/assignments") {
        return Response.json([
          {
            id: "asg-demo",
            kind: "homework",
            subject: "математика",
            state: "issued",
            deadline: "2026-12-01T18:00:00Z",
            answered: 0,
            total: opts.summaryTotal ?? 1,
            student_id: "student-demo",
          },
        ]);
      }
      if (method === "GET" && path === "/assignments/asg-demo") {
        return Response.json(opts.assignment);
      }
      if (method === "POST" && path === "/assignments/asg-demo/submit") {
        const raw = await readBody(input, init);
        const parsed = raw
          ? (JSON.parse(raw) as { confirm_unanswered?: boolean })
          : {};
        if (parsed.confirm_unanswered === true) {
          return Response.json({ assignment_id: "asg-demo", items: [] });
        }
        return Response.json(
          {
            code: "unanswered_confirmation_required",
            message: "unanswered_confirmation_required",
            details: { left: 4 },
          },
          { status: 409 },
        );
      }
      if (
        method === "POST" &&
        path === "/assignments/asg-demo/items/item-1/answer"
      ) {
        if (opts.online && !opts.online()) {
          throw new TypeError("Failed to fetch");
        }
        sentBodies.push(await readBody(input, init));
        return Response.json(
          opts.answer ?? {
            status: "scored",
            item_state: "open",
            attempts_left: 1,
            verdict: "wrong",
          },
        );
      }
      if (method === "GET" && path === "/lessons/lesson-demo") {
        return Response.json(lessonCard());
      }
      if (method === "GET" && path === "/lessons/lesson-demo/conspect") {
        return Response.json(opts.conspect);
      }
      if (method === "GET" && path === "/lessons/lesson-demo/conspect.pdf") {
        return Response.json(
          {
            code: "pdf_unavailable",
            message: STACK,
            details: { key: KEY, solution_block: SOLUTION },
          },
          { status: 503 },
        );
      }
      return Response.json({ code: "forbidden" }, { status: 404 });
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderAt(path: string) {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={[path]}>
        <App />
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
    records.push({
      url: asUrl(input).href,
      method: asMethod(input, init),
      body: await readBody(input, init),
    });
  }
  return records;
}

function answerPosts(records: CallRecord[]) {
  return records.filter(
    (record) =>
      record.method === "POST" &&
      record.url.includes("/assignments/asg-demo/items/item-1/answer"),
  );
}

function renderedText(): string {
  const values = [...document.querySelectorAll("input, textarea, select")].map(
    (node) => (node as HTMLInputElement).value,
  );
  return `${document.body.textContent ?? ""}\n${values.join("\n")}`;
}

function expectNoLeak() {
  const text = renderedText();
  expect(text).not.toContain(KEY);
  expect(text).not.toContain(SOLUTION);
  expect(text).not.toContain(MARK);
  expect(text.toLowerCase()).not.toContain("верная");
}

test("wb-r4: открытая задача без ключа и пометки верной опции", async () => {
  installFetch({ assignment: openChoiceAssignment() });
  renderAt("/assignment/asg-demo");

  expect(await screen.findByText(STATEMENT)).toBeTruthy();
  expect(renderedText()).toContain("не отвечено");
  expectNoLeak();

  cleanup();
  renderAt("/task/asg-demo/item-1");

  expect(await screen.findByText(STATEMENT)).toBeTruthy();
  expect(await screen.findByRole("radio", { name: /север/ })).toBeTruthy();
  expectNoLeak();
});

test("wb-r15: часть В, первая ошибка — «неверно» и «осталась 1 попытка», без ключа", async () => {
  const fetchMock = installFetch({
    assignment: integerAssignment(),
    answer: {
      status: "scored",
      item_state: "open",
      attempts_left: 1,
      verdict: "wrong",
      points: 0,
      max_points: 2,
    },
  });
  renderAt("/task/asg-demo/item-1");

  fireEvent.change(await screen.findByRole("textbox"), {
    target: { value: "13" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));

  await waitFor(async () => {
    const posts = answerPosts(await callRecords(fetchMock));
    expect(posts).toHaveLength(1);
    const body = JSON.parse(posts[0]?.body ?? "") as AnswerBody;
    expect(body.answer).toBe("13");
    const text = renderedText().toLowerCase();
    expect(text).toContain("неверно");
    expect(text).toContain("осталась 1 попытка");
  });
  expect(renderedText().toLowerCase()).not.toContain("баллы");
  expectNoLeak();
});

test("wb-r16: четыре задачи без ответа — подтверждение «осталось 4»", async () => {
  const confirmSpy = vi
    .spyOn(window, "confirm")
    .mockImplementation(() => false);
  installFetch({
    assignment: fourOpenAssignment(),
    summaryTotal: 4,
  });
  renderAt("/assignment/asg-demo");

  expect(await screen.findByRole("button", { name: "Сдать" })).toBeTruthy();
  for (const statement of TASKS) {
    expect(renderedText()).toContain(statement);
  }
  expect(renderedText()).toContain("не отвечено");
  expect(renderedText()).not.toContain("осталось 4");

  fireEvent.click(screen.getByRole("button", { name: "Сдать" }));

  await waitFor(() => {
    const fromPage = renderedText().includes("осталось 4");
    const fromDialog = confirmSpy.mock.calls.some((call) =>
      String(call[0] ?? "").includes("осталось 4"),
    );
    expect(fromPage || fromDialog).toBe(true);
  });
  expect(renderedText()).not.toContain("unanswered_confirmation_required");
  expectNoLeak();
});

test("wb-r18: нет связи — «не отправлено», после связи один ответ", async () => {
  let online = true;
  const sentBodies: string[] = [];
  installFetch({
    assignment: integerAssignment(),
    online: () => online,
    sentBodies,
    answer: {
      status: "scored",
      item_state: "open",
      attempts_left: 1,
      verdict: "wrong",
      points: 0,
      max_points: 2,
    },
  });
  renderAt("/task/asg-demo/item-1");

  const field = await screen.findByRole("textbox");
  fireEvent.change(field, { target: { value: "7" } });
  online = false;
  setOnline(false);
  window.dispatchEvent(new Event("offline"));
  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));

  await waitFor(() => {
    expect(renderedText()).toContain("не отправлено");
  });
  expect(renderedText()).not.toContain("Failed to fetch");
  expect(renderedText()).not.toContain("TypeError");
  expect(sentBodies).toHaveLength(0);
  expectNoLeak();

  const again = screen.queryByRole("button", { name: "Ответить" });
  if (again) {
    fireEvent.click(again);
  }
  expect(sentBodies).toHaveLength(0);

  online = true;
  setOnline(true);
  window.dispatchEvent(new Event("online"));

  await waitFor(() => {
    expect(sentBodies).toHaveLength(1);
  });
  window.dispatchEvent(new Event("online"));
  await new Promise((resolve) => {
    setTimeout(resolve, 50);
  });
  expect(sentBodies).toHaveLength(1);
  const body = JSON.parse(sentBodies[0] ?? "") as Record<string, unknown>;
  expect(Object.keys(body).sort()).toEqual(["answer", "idempotency_key"]);
  expect(body.answer).toBe("7");
  expect(typeof body.idempotency_key).toBe("string");
  expect(body.idempotency_key).not.toBe("");
  expect(JSON.stringify(body)).not.toContain("Рысь");
});

test("wb-r26: блок со script не попадает в страницу", async () => {
  installFetch({
    conspect: conspect([
      { id: "b-ok", markdown: CLEAN_BLOCK },
      {
        id: "b-bad",
        markdown: `<script>alert("${SCRIPT_PAYLOAD}")</script>`,
      },
    ]),
  });
  renderAt("/lesson/lesson-demo");

  expect(await screen.findByText(CLEAN_BLOCK)).toBeTruthy();
  expect(document.querySelector("script")).toBeNull();
  expect(document.body.innerHTML.toLowerCase()).not.toContain("<script");
  expect(renderedText()).not.toContain(SCRIPT_PAYLOAD);
  expectNoLeak();
});

test("wb-r28: «Скачать PDF», ответ 503 без ключа и стека", async () => {
  const fetchMock = installFetch({
    conspect: conspect([{ id: "b-ok", markdown: CLEAN_BLOCK }]),
  });
  renderAt("/lesson/lesson-demo");

  expect(await screen.findByText(CLEAN_BLOCK)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Скачать PDF" }));

  await waitFor(async () => {
    const records = await callRecords(fetchMock);
    expect(
      records.some(
        (record) =>
          record.method === "GET" &&
          record.url.includes("/lessons/lesson-demo/conspect.pdf"),
      ),
    ).toBe(true);
  });
  expect(renderedText()).toContain(CLEAN_BLOCK);
  expectNoLeak();
  const text = renderedText();
  expect(text).not.toContain("goroutine");
  expect(text).not.toContain("stack.go");
  expect(text).not.toContain("runtime.stack");
});
