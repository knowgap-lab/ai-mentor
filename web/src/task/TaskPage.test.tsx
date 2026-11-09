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

// TaskPage читает маршрут /task/:assignmentId/:itemId, условие берёт из
// GET /assignments/{id} (AssignmentStudentView) и шлёт
// POST /assignments/{id}/items/{item}/answer (AnswerRequest в core-api.yaml).
// ВОПРОС: откуда страница берёт X-CSRF-Token? В core-api.yaml у схемы Me нет поля токена и нет отдельного пути. Тест заголовок не проверяет.

const STATEMENT = "Найдите значение выражения.";

type AnswerBody = {
  answer?: unknown;
  idempotency_key?: unknown;
};

type CallRecord = {
  url: string;
  method: string;
  body: string;
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function choiceAssignment() {
  return {
    id: "asg-1",
    kind: "homework",
    state: "issued",
    items: [
      {
        item_id: "item-1",
        statement: STATEMENT,
        answer_format: "ct.choice1",
        item_state: "open",
        attempts_left: 1,
        options: [
          { n: 1, text: "десять" },
          { n: 2, text: "одиннадцать" },
          { n: 3, text: "двенадцать" },
          { n: 4, text: "тринадцать" },
          { n: 5, text: "четырнадцать" },
        ],
      },
    ],
  };
}

function integerAssignment() {
  return {
    id: "asg-1",
    kind: "homework",
    state: "issued",
    items: [
      {
        item_id: "item-1",
        statement: STATEMENT,
        answer_format: "ct.integer",
        item_state: "open",
        attempts_left: 2,
      },
    ],
  };
}

function installFetch(assignment: unknown, answer?: unknown) {
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : String(input);
      const method = (
        init?.method ?? (input instanceof Request ? input.method : "GET")
      ).toUpperCase();
      if (method === "GET" && url.includes("/assignments/asg-1")) {
        return Response.json(assignment);
      }
      if (method === "POST" && url.includes("/items/item-1/answer")) {
        return Response.json(
          answer ?? { status: "scored", item_state: "open" },
        );
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
      <MemoryRouter initialEntries={["/task/asg-1/item-1"]}>
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
    records.push({ url, method, body });
  }
  return records;
}

function answerPosts(records: CallRecord[]) {
  return records.filter(
    (record) =>
      record.method === "POST" &&
      record.url.includes("/assignments/asg-1/items/item-1/answer"),
  );
}

function expectSingleAnswer(records: CallRecord[], answer: string | number[]) {
  const posts = answerPosts(records);
  expect(posts).toHaveLength(1);
  const body = JSON.parse(posts[0]?.body ?? "") as AnswerBody;
  expect(body.answer).toEqual(answer);
  expect(typeof body.idempotency_key).toBe("string");
  expect(body.idempotency_key).not.toBe("");
}

function pageText() {
  return document.body.textContent ?? "";
}

function expectNoKey() {
  const text = pageText();
  expect(text).not.toContain("12");
  expect(text.toLowerCase()).not.toContain("решение");
}

test("wb-r10: ct.choice1 отправляет выбранный номер", async () => {
  const fetchMock = installFetch(choiceAssignment(), {
    status: "scored",
    item_state: "final",
    attempts_left: 0,
    verdict: "correct",
    points: 1,
    max_points: 1,
  });
  renderTask();

  expect(await screen.findAllByRole("radio")).toHaveLength(5);
  for (const n of [1, 2, 3, 4, 5]) {
    expect(
      screen.getByRole("radio", { name: new RegExp(`(?:^|\\D)${n}(?:\\D|$)`) }),
    ).toBeTruthy();
  }

  fireEvent.click(screen.getByRole("radio", { name: /(?:^|\D)3(?:\D|$)/ }));
  expect(answerPosts(await callRecords(fetchMock))).toHaveLength(0);

  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));
  await waitFor(async () => {
    expectSingleAnswer(await callRecords(fetchMock), [3]);
  });
});

test("wb-r14: ct.integer не отправляет «12,0» и показывает «введите целое число»", async () => {
  const fetchMock = installFetch(integerAssignment());
  renderTask();

  const field = await screen.findByRole("textbox");
  fireEvent.change(field, { target: { value: "12,0" } });
  expect(pageText()).toContain("введите целое число");

  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));
  const records = await callRecords(fetchMock);
  expect(records.some((record) => record.body.includes("12,0"))).toBe(false);
  expect(answerPosts(records)).toHaveLength(0);
});

test("wb-r4: ответ без show не содержит ключ 12", async () => {
  const fetchMock = installFetch(integerAssignment(), {
    status: "scored",
    item_state: "final",
    attempts_left: 0,
    verdict: "correct",
    points: 2,
    max_points: 2,
  });
  renderTask();

  await screen.findByText(STATEMENT);
  expectNoKey();

  fireEvent.change(await screen.findByRole("textbox"), {
    target: { value: "7" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));

  await waitFor(async () => {
    expectSingleAnswer(await callRecords(fetchMock), "7");
    expect(screen.getByText(/^верно$/i)).toBeTruthy();
  });
  expectNoKey();
});

test("wb-r4: ответ с show.key показывает ключ 12", async () => {
  const fetchMock = installFetch(integerAssignment(), {
    status: "scored",
    item_state: "final",
    attempts_left: 0,
    verdict: "wrong",
    points: 0,
    max_points: 2,
    show: { key: "12" },
  });
  renderTask();

  await screen.findByText(STATEMENT);
  expectNoKey();

  fireEvent.change(await screen.findByRole("textbox"), {
    target: { value: "14" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));

  await waitFor(async () => {
    expectSingleAnswer(await callRecords(fetchMock), "14");
    expect(pageText()).toContain("12");
  });
});

test("wb-r15: часть В, первая ошибка — «неверно» и «осталась 1 попытка», без ключа", async () => {
  const fetchMock = installFetch(integerAssignment(), {
    status: "scored",
    item_state: "open",
    attempts_left: 1,
    verdict: "wrong",
    points: 0,
    max_points: 2,
  });
  renderTask();

  fireEvent.change(await screen.findByRole("textbox"), {
    target: { value: "13" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Ответить" }));

  await waitFor(async () => {
    expectSingleAnswer(await callRecords(fetchMock), "13");
    const text = pageText().toLowerCase();
    expect(text).toContain("неверно");
    expect(text).toContain("осталась 1 попытка");
  });
  expectNoKey();
});
