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

// Экран /tutor. Подмена fetch, сервер не нужен.
// POST /auth/dev {telegram_id, role}; GET /students; GET /assignments?student_id=;
// GET /assignments/{id}/attempts (AttemptView: raw_input, verdict); POST /auth/logout.
// Чужого ученика в GET /students нет (D108). В попытку подмешаны key, show.key
// и пометка «верная опция»: в AttemptView их нет (WB-R4, WB-R22).
// web-02: на экране попыток нет ключа и пометки верной опции.
// web-14: после входа виден экран «Ученики», ближайший урок раньше.
// Кнопки «показать» на попытках нет. Пример web-15 (экран «Подготовка») здесь не открывается.
// ВОПРОС: какая подпись у поля telegram_id; варианты: доступное имя telegram_id / одно текстовое поле с любой подписью. Тест принимает оба и не фиксирует русскую подпись: в web.md её нет.
// ВОПРОС: кто ставит роль tutor в форме; варианты: экран сам отправляет tutor / человек выбирает. Тест контрол роли не переключает и проверяет тело POST /auth/dev.
// ВОПРОС: каким русским словом показывать verdict wrong на попытках репетитора; варианты: «неверно» как на странице задачи (WB-R15) / другая подпись. Тест текст вердикта не проверяет.
// ВОПРОС: откуда страница берёт X-CSRF-Token для POST /auth/logout; варианты: заголовок ответа POST /auth/dev (в схеме Me поля нет) / иначе. Тест заголовок не проверяет.
// ВОПРОС: писать ли на попытках до сдачи «ещё не сдано» (WB-R24); варианты: писать / не писать на этом экране. Тест это не проверяет.
// ВОПРОС: какими фразами показывать показатели в списке учеников; варианты: «ДЗ ждёт просмотра», «сдано», «просрочено», «открытые споры», «не доставлено» / короче. Тест показатели не проверяет.
// ВОПРОС: как в этом прогоне проверять узкий экран WB-R1; варианты: ширина 360 и кнопки от 44 px в браузере / не в jsdom. Тест ширину не проверяет.

const TELEGRAM_ID = "tg-demo-tutor";
const OWN_NEAR = "Лиса";
const OWN_FAR = "Волк";
const FOREIGN = "Барс";
const RAW = "7";
const KEY = "12";
const SOLUTION = "ct-math:M10.B09";

type CallRecord = {
  url: string;
  method: string;
  body: string;
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function asUrl(input: RequestInfo | URL): URL {
  const raw = input instanceof Request ? input.url : String(input);
  return new URL(raw, "http://localhost");
}

function asMethod(input: RequestInfo | URL, init?: RequestInit): string {
  const fromInit = init?.method;
  const fromRequest = input instanceof Request ? input.method : "GET";
  return (fromInit ?? fromRequest).toUpperCase();
}

function assignmentSummary(id: string, studentId: string) {
  return {
    id,
    kind: "homework",
    subject: "ct-math",
    state: "issued",
    total: 1,
    student_id: studentId,
  };
}

function assignmentView(id: string) {
  return {
    id,
    kind: "homework",
    state: "issued",
    items: [
      {
        item_id: "item-1",
        statement: "Найдите значение выражения.",
        answer_format: "ct.integer",
        item_state: "open",
        attempts_left: 1,
      },
    ],
  };
}

function studentCard(id: string, displayName: string) {
  return {
    id,
    display_name: displayName,
    status: "active",
    consent: "given",
  };
}

function attemptLeak() {
  return {
    attempt_id: "att-1",
    item_id: "item-1",
    attempt_no: 1,
    raw_input: RAW,
    verdict: "wrong",
    points: 0,
    submitted: false,
    key: KEY,
    show: { key: KEY, solution_block: SOLUTION },
    options: [{ n: 1, text: "десять", mark: "верная опция" }],
  };
}

function installFetch() {
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = asUrl(input);
      const method = asMethod(input, init);
      const path = url.pathname.replace(/\/$/, "") || "/";
      if (method === "POST" && path.endsWith("/auth/dev")) {
        return Response.json({ user_id: "tutor-demo", role: "tutor" });
      }
      if (method === "POST" && path.endsWith("/auth/logout")) {
        return new Response(null, { status: 204 });
      }
      if (method === "GET" && path.endsWith("/assignments/asg-1/attempts")) {
        return Response.json([attemptLeak()]);
      }
      if (method === "GET" && path.endsWith("/assignments/asg-2/attempts")) {
        return Response.json([]);
      }
      if (method === "GET" && path.endsWith("/assignments/asg-1")) {
        return Response.json(assignmentView("asg-1"));
      }
      if (method === "GET" && path.endsWith("/assignments/asg-2")) {
        return Response.json(assignmentView("asg-2"));
      }
      if (method === "GET" && path.endsWith("/assignments")) {
        const studentId = url.searchParams.get("student_id");
        if (studentId === "student-demo") {
          return Response.json([assignmentSummary("asg-1", "student-demo")]);
        }
        if (studentId === "student-wolf") {
          return Response.json([assignmentSummary("asg-2", "student-wolf")]);
        }
        return Response.json([]);
      }
      if (path.includes("student-other")) {
        return Response.json({ code: "forbidden" }, { status: 403 });
      }
      if (method === "GET" && path.endsWith("/students/student-demo")) {
        return Response.json(studentCard("student-demo", OWN_NEAR));
      }
      if (method === "GET" && path.endsWith("/students/student-wolf")) {
        return Response.json(studentCard("student-wolf", OWN_FAR));
      }
      if (method === "GET" && path.endsWith("/students")) {
        return Response.json([
          {
            id: "student-wolf",
            display_name: OWN_FAR,
            next_lesson_at: "2026-11-20T08:00:00Z",
          },
          {
            id: "student-demo",
            display_name: OWN_NEAR,
            next_lesson_at: "2026-11-03T08:00:00Z",
          },
        ]);
      }
      return Response.json({ code: "forbidden" }, { status: 404 });
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderTutor() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={["/tutor"]}>
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
    const url = asUrl(input).href;
    const method = asMethod(input, init);
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

function renderedText(): string {
  const values = [...document.querySelectorAll("input, textarea, select")].map(
    (node) => (node as HTMLInputElement).value,
  );
  return `${document.body.textContent ?? ""}\n${values.join("\n")}`;
}

function expectNoKey() {
  const text = renderedText();
  expect(text).not.toContain(KEY);
  expect(text).not.toContain(SOLUTION);
  expect(text).not.toContain("верная");
  expect(screen.queryByRole("button", { name: /показать/i })).toBeNull();
  expect(screen.queryByRole("link", { name: /показать/i })).toBeNull();
}

function telegramField(): HTMLElement {
  return (
    screen.queryByRole("textbox", { name: /telegram_id/i }) ??
    screen.getByRole("textbox")
  );
}

function exitControl(): HTMLElement {
  return (
    screen.queryByRole("button", { name: /^выход$/i }) ??
    screen.getByRole("link", { name: /^выход$/i })
  );
}

async function login(fetchMock: ReturnType<typeof vi.fn>) {
  fireEvent.change(telegramField(), { target: { value: TELEGRAM_ID } });
  fireEvent.click(screen.getByRole("button", { name: "Войти" }));
  await waitFor(() => {
    expect(screen.getByText(OWN_NEAR)).toBeTruthy();
  });
  const posts = (await callRecords(fetchMock)).filter(
    (record) => record.method === "POST" && record.url.includes("/auth/dev"),
  );
  expect(posts).toHaveLength(1);
  expect(JSON.parse(posts[0]?.body ?? "")).toEqual({
    telegram_id: TELEGRAM_ID,
    role: "tutor",
  });
}

test("wb-r4: свой ученик виден, чужой не виден", async () => {
  const fetchMock = installFetch();
  renderTutor();

  expect(await screen.findByRole("button", { name: "Войти" })).toBeTruthy();
  expect(screen.queryByText(OWN_NEAR)).toBeNull();
  expect(screen.queryByText(OWN_FAR)).toBeNull();
  expect(screen.queryByText(FOREIGN)).toBeNull();
  expect(
    (await callRecords(fetchMock)).some((record) =>
      record.url.includes("/auth/dev"),
    ),
  ).toBe(false);

  await login(fetchMock);

  const text = renderedText();
  expect(text).toContain("Ученики");
  expect(text).toContain(OWN_NEAR);
  expect(text).toContain(OWN_FAR);
  expect(text).not.toContain(FOREIGN);
  expect(text.indexOf(OWN_NEAR)).toBeLessThan(text.indexOf(OWN_FAR));
  expect(screen.queryByText(FOREIGN)).toBeNull();
  const records = await callRecords(fetchMock);
  expect(
    records.some(
      (record) =>
        record.method === "GET" &&
        new URL(record.url).pathname.replace(/\/$/, "").endsWith("/students"),
    ),
  ).toBe(true);
  expect(records.some((record) => record.url.includes("student-other"))).toBe(
    false,
  );
  expectNoKey();
});

test("wb-r4: на экране попыток нет ключа 12", async () => {
  const fetchMock = installFetch();
  renderTutor();
  expect(await screen.findByRole("button", { name: "Войти" })).toBeTruthy();
  await login(fetchMock);

  expect(renderedText()).not.toContain(RAW);
  fireEvent.click(screen.getByText(OWN_NEAR));

  await waitFor(() => {
    expect(renderedText()).toContain(RAW);
  });
  expectNoKey();
  const records = await callRecords(fetchMock);
  expect(
    records.some(
      (record) =>
        record.method === "GET" &&
        record.url.includes("student_id=student-demo"),
    ),
  ).toBe(true);
  expect(
    records.some(
      (record) =>
        record.method === "GET" &&
        record.url.includes("/assignments/asg-1/attempts"),
    ),
  ).toBe(true);
});

test("wb-r4: выход возвращает к форме входа", async () => {
  const fetchMock = installFetch();
  renderTutor();
  expect(await screen.findByRole("button", { name: "Войти" })).toBeTruthy();
  await login(fetchMock);

  fireEvent.click(exitControl());

  await waitFor(async () => {
    expect(screen.getByRole("button", { name: "Войти" })).toBeTruthy();
    const logouts = (await callRecords(fetchMock)).filter(
      (record) =>
        record.method === "POST" && record.url.includes("/auth/logout"),
    );
    expect(logouts).toHaveLength(1);
  });
  expect(screen.queryByText(OWN_NEAR)).toBeNull();
  expect(screen.queryByText(OWN_FAR)).toBeNull();
  expect(screen.queryByText(FOREIGN)).toBeNull();
});
