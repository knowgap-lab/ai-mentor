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
import { StudentCardPage } from "./StudentCardPage";

// Пакет W5, экран «Ученик» (web.md, карточка).
// Маршрут теста: /tutor/students/:studentId. Ключ реестра — «Репетитор/Ученик».
// web/screens.json этот файл не заполняет.
// GET /students/{id} — Student.
// GET /students/{id}/plans/{subject} — PlanView, subject из Student.subjects.
// GET /assignments?student_id= — AssignmentSummary.
// GET /assignments/{id}/attempts — AttemptView. В ответ подмешаны key и «верная опция»:
// в AttemptView их нет, на этом экране их нет (WB-R4). Экран «Подготовка» здесь не открывается.
// GET /students/{id}/discuss-list — DiscussItem, список «разобрать на уроке» (AT-R17).
// POST /students/{id}/archive и /unarchive (PV-R22).
// POST /students/{id}/invites (AU-R8). POST /students/{id}/consent-links, kind parent:
// дата рождения 2010-04-02, на 2026-11-15 ученику нет 18 (PV-R12).
// POST /students/{id}/consent/cancel-revoke — кнопка «отменить отзыв», только если consent revoked (WB-R9).
// ВОПРОС: чтения правила расписания в core-api.yaml нет, только PUT. Тест сохранение не нажимает
// и GET расписания не ждёт. Заголовок «Расписание» обязателен. Время занятия берётся из starts_at.
// ВОПРОС: русские слова для planned, frozen и held в web.md не заданы. Тест их не фиксирует.
// «пропущен» — PL-R4a, «перенесено» и «добавлено» — PL-R8.
// ВОПРОС: подпись вида выдачи homework не задана. Тест требует срок в Europe/Minsk (WB-R2).
// ВОПРОС: слово подтверждения архивации не задано. После 204 тест ждёт кнопку «Вернуть» (PV-R22).
// ВОПРОС: «ещё не сдано» — экран «Итоги ДЗ» (WB-R24), не карточка. Тест эту фразу не требует.
// Сырой ввод виден (D81).
// ВОПРОС: клик по имени на «Ученики» заморожен и этот файл его не меняет.
// ВОПРОС: слова «загрузка» и «пусто» не заданы.
// ВОПРОС: текста для consent_kind_invalid в таблице web.md нет. Тест этот код не шлёт.
// Дата рождения на экран не требуется: в составе карточки её нет. В фикстуре она нужна для kind.

const CSRF = "csrf-w5";
const STUDENT = "stu-w5";
const NAME = "Ольха";
const HIDDEN = "key-hidden-w5";
const KEY = "918273";
const SOLUTION = "block-w5-hidden";
const MARK = "верная опция";
const RAW = "8417";
const LESSON_MOVED = "Выдуманные дроби";
const LESSON_SKIPPED = "Выдуманные степени";
const TOPIC = "Выдуманная тема";
const KIND = "лекция";
const VARIANT = "var-w5";
const INVITE_URL = "https://example.test/invite/w5-demo";
const CONSENT_URL = "https://example.test/consent/w5-demo";
const WARN_VARIANTS = "У Ольха нет новых вариантов типа Выдуманный тип";
const WARN_PLAN = "Плану не хватает 2 уроков до экзамена у Ольха";
const WARN_CONSENT = "Нужно новое согласие: Ольха, срок 2027-03-01";

const LOAD_ERRORS: [string, number, string][] = [
  ["forbidden", 403, "Нет доступа или объекта не существует"],
  ["unavailable", 503, "Нет связи, попробуйте ещё раз"],
  ["login_stale", 401, "Войдите ещё раз"],
];

const INVITE_ERRORS: [string, number, string][] = [
  ["consent_missing", 423, "Нужно согласие родителя — обратитесь к репетитору"],
  ["account_locked", 423, "Вход закрыт — обратитесь к репетитору"],
  ["forbidden", 403, "Нет доступа или объекта не существует"],
  ["csrf", 403, "Обновите страницу и повторите"],
  ["rate_limited", 429, "Слишком часто, подождите немного"],
  ["unavailable", 503, "Нет связи, попробуйте ещё раз"],
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

function warnings() {
  return [
    { type: "variants_needed", text: WARN_VARIANTS },
    { type: "plan_does_not_fit", text: WARN_PLAN },
    { type: "consent_outdated", text: WARN_CONSENT },
  ];
}

function studentBody(status = "active", consent = "given") {
  return {
    id: STUDENT,
    display_name: NAME,
    birth_date: "2010-04-02",
    status,
    consent,
    telegram_bound: false,
    subjects: ["ct-math"],
    warnings: warnings(),
    key: HIDDEN,
  };
}

function emptyStudent() {
  return {
    id: STUDENT,
    display_name: NAME,
    birth_date: "2010-04-02",
    status: "active",
    consent: "given",
    subjects: [],
    warnings: [],
  };
}

function planBody() {
  return {
    lessons: [
      {
        id: "lesson-w5-moved",
        template_id: "tpl-w5-1",
        place_no: 12,
        title: LESSON_MOVED,
        kind: KIND,
        topics: [TOPIC],
        state: "planned",
        starts_at: "2026-11-03T15:00:00Z",
        moved: true,
        added: false,
      },
      {
        id: "lesson-w5-skipped",
        template_id: "tpl-w5-2",
        place_no: 13,
        title: LESSON_SKIPPED,
        state: "skipped",
        planned_week: "2026-W46",
        moved: false,
        added: true,
      },
    ],
    warnings: [],
  };
}

function assignmentBody() {
  return {
    id: "asg-w5",
    kind: "homework",
    subject: "ct-math",
    state: "issued",
    deadline: "2026-11-20T06:30:00Z",
    answered: 2,
    total: 6,
    student_id: STUDENT,
  };
}

function attemptLeak() {
  return {
    attempt_id: "att-w5",
    item_id: "item-w5",
    attempt_no: 1,
    raw_input: RAW,
    verdict: "wrong",
    points: 0,
    submitted: false,
    key: KEY,
    show: { key: KEY, solution_block: SOLUTION },
    options: [{ n: 1, text: "десять", mark: MARK }],
  };
}

function discussBody() {
  return [
    {
      item_id: "item-w5",
      variant_id: VARIANT,
      assignment_id: "asg-w5",
    },
  ];
}

function errorBody(code: string) {
  return { code, message: "goroutine 7 [running]", details: { key: HIDDEN } };
}

function link(url: string) {
  return { url, expires_at: "2026-11-29T12:00:00Z" };
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

function happyReply(method: string, path: string): Response | "network" {
  if (method === "GET" && path === `/students/${STUDENT}`) {
    return Response.json(studentBody());
  }
  if (method === "GET" && path === `/students/${STUDENT}/plans/ct-math`) {
    return Response.json(planBody());
  }
  if (method === "GET" && path === `/students/${STUDENT}/discuss-list`) {
    return Response.json(discussBody());
  }
  if (method === "GET" && path === "/assignments") {
    return Response.json([assignmentBody()]);
  }
  if (method === "GET" && path === "/assignments/asg-w5/attempts") {
    return Response.json([attemptLeak()]);
  }
  return Response.json({ code: "forbidden" }, { status: 404 });
}

function renderCard() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/tutor/students/${STUDENT}`]}>
        <Routes>
          <Route
            path="/tutor/students/:studentId"
            element={<StudentCardPage />}
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
  for (const node of document.body.querySelectorAll("style")) {
    node.textContent = "";
  }
  const values = [...document.querySelectorAll("input, textarea, select")].map(
    (node) => (node as HTMLInputElement).value,
  );
  return `${document.body.textContent ?? ""}\n${values.join("\n")}`;
}

function expectNoSecrets(text: string) {
  expect(text).not.toContain(HIDDEN);
  expect(text).not.toContain(KEY);
  expect(text).not.toContain(SOLUTION);
  expect(text).not.toContain(MARK);
  expect(text).not.toContain("верная");
  expect(screen.queryByRole("button", { name: /показать/i })).toBeNull();
}

test("w5: маршрут карточки в приложении", async () => {
  installFetch(happyReply);
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/tutor/students/${STUDENT}`]}>
        <App />
      </MemoryRouter>
    </MantineProvider>,
  );
  expect(await screen.findByRole("heading", { name: /^Ученик$/ })).toBeTruthy();
  expect(await screen.findByText(NAME)).toBeTruthy();
});

test("w5: карточка собирает состав экрана и не показывает ключ", async () => {
  const fetchMock = installFetch(happyReply);
  renderCard();

  expect(await screen.findByRole("heading", { name: /^Ученик$/ })).toBeTruthy();
  expect(await screen.findByText(LESSON_MOVED)).toBeTruthy();
  expect(screen.getByText(LESSON_SKIPPED)).toBeTruthy();
  expect(await screen.findByText(RAW)).toBeTruthy();

  const text = pageText();
  expect(text).toContain(NAME);
  expect(text).toContain("Расписание");
  expect(text).toContain("Выдачи");
  expect(text).toContain("разобрать на уроке");
  expect(text).toContain("Согласие");
  expect(text).toContain(KIND);
  expect(text).toContain(TOPIC);
  expect(text).toContain("12");
  expect(text).toContain("13");
  expect(text).toContain("перенесено");
  expect(text).toContain("добавлено");
  expect(text).toContain("пропущен");
  expect(text).toContain("2026-W46");
  expect(text).toContain("18:00");
  expect(text).toContain("09:30");
  expect(text).not.toContain("15:00:00Z");
  expect(text).not.toContain("06:30:00Z");
  expect(text).toContain(VARIANT);
  expect(text).toContain(WARN_VARIANTS);
  expect(text).toContain(WARN_PLAN);
  expect(text).toContain(WARN_CONSENT);
  const scheduleAt = text.indexOf("Расписание");
  expect(text.indexOf(WARN_VARIANTS)).toBeLessThan(scheduleAt);
  expect(text.indexOf(WARN_PLAN)).toBeLessThan(scheduleAt);
  expect(text.indexOf(WARN_CONSENT)).toBeLessThan(scheduleAt);
  expect(text).not.toContain("variants_needed");
  expect(text).not.toContain("plan_does_not_fit");
  expect(text).not.toContain("consent_outdated");
  expectNoSecrets(text);
  expect(screen.getByRole("button", { name: /^архив$/i })).toBeTruthy();
  expect(screen.getByRole("button", { name: /^приглашение$/i })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: /^ссылка согласия$/i }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: /^отменить отзыв$/i }),
  ).toBeNull();
  expect(text).not.toContain(INVITE_URL);

  const records = await callRecords(fetchMock);
  expect(
    records.some(
      (record) =>
        record.method === "GET" &&
        record.url.includes(`/students/${STUDENT}/plans/ct-math`),
    ),
  ).toBe(true);
  expect(
    records.some((record) => {
      if (record.method !== "GET") {
        return false;
      }
      const url = new URL(record.url, "http://localhost");
      return (
        url.pathname.replace(/\/$/, "") === "/assignments" &&
        url.searchParams.get("student_id") === STUDENT
      );
    }),
  ).toBe(true);
  expect(
    records.some(
      (record) =>
        record.method === "GET" &&
        record.url.includes("/assignments/asg-w5/attempts"),
    ),
  ).toBe(true);
  expect(
    records.some(
      (record) =>
        record.method === "GET" &&
        record.url.includes(`/students/${STUDENT}/discuss-list`),
    ),
  ).toBe(true);
  expect(records.some((record) => record.url.includes("/preparation"))).toBe(
    false,
  );
});

test("w5: пустая карточка без ключа и без чужих кнопок", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/students/${STUDENT}`) {
      return Response.json(emptyStudent());
    }
    if (method === "GET" && path === "/assignments") {
      return Response.json([]);
    }
    if (method === "GET" && path === `/students/${STUDENT}/discuss-list`) {
      return Response.json([]);
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderCard();

  expect(await screen.findByRole("heading", { name: /^Ученик$/ })).toBeTruthy();
  expect(await screen.findByText(NAME)).toBeTruthy();
  const text = pageText();
  expect(text).toContain("Расписание");
  expect(text).toContain("Выдачи");
  expect(text).toContain("разобрать на уроке");
  expect(text).not.toContain(LESSON_MOVED);
  expect(text).not.toContain(RAW);
  expect(text).not.toContain(VARIANT);
  expectNoSecrets(text);
});

test("wb-r30: карточка без сети — «Нет связи» и «Повторить»", async () => {
  let online = false;
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === `/students/${STUDENT}`) {
      if (!online) {
        return "network";
      }
      return Response.json(studentBody());
    }
    if (online) {
      return happyReply(method, path);
    }
    return "network";
  });
  renderCard();

  expect(await screen.findByText("Нет связи")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Повторить" })).toBeTruthy();
  expect(pageText()).not.toContain("попробуйте ещё раз");
  expect(pageText()).not.toContain("Failed to fetch");
  expect(pageText()).not.toContain("TypeError");
  expectNoSecrets(pageText());

  online = true;
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(await screen.findByText(RAW)).toBeTruthy();
  const gets = (await callRecords(fetchMock)).filter((record) => {
    if (record.method !== "GET") {
      return false;
    }
    const url = new URL(record.url, "http://localhost");
    return url.pathname.replace(/\/$/, "") === `/students/${STUDENT}`;
  });
  expect(gets.length).toBeGreaterThanOrEqual(2);
});

test.each(LOAD_ERRORS)(
  "wb-r6: загрузка карточки, код %s",
  async (code, status, sentence) => {
    installFetch((method, path) => {
      if (method === "GET" && path === `/students/${STUDENT}`) {
        return Response.json(errorBody(code), { status });
      }
      return Response.json({ code: "forbidden" }, { status: 404 });
    });
    renderCard();

    expect(await screen.findByText(sentence)).toBeTruthy();
    const text = pageText();
    expect(text).not.toContain(code);
    expect(text).not.toContain("goroutine");
    expectNoSecrets(text);
  },
);

test("pv-r22: «Архив» архивирует, затем видна «Вернуть»", async () => {
  let archived = false;
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/students/${STUDENT}/archive`) {
      archived = true;
      return new Response(null, { status: 204 });
    }
    if (method === "GET" && path === `/students/${STUDENT}`) {
      return Response.json(
        studentBody(archived ? "archived" : "active", "given"),
      );
    }
    return happyReply(method, path);
  });
  renderCard();

  fireEvent.click(await screen.findByRole("button", { name: /^архив$/i }));
  expect(
    await screen.findByRole("button", { name: /^вернуть$/i }),
  ).toBeTruthy();
  const posts = (await callRecords(fetchMock)).filter(
    (record) =>
      record.method === "POST" &&
      record.url.includes(`/students/${STUDENT}/archive`),
  );
  expect(posts).toHaveLength(1);
  expect(posts[0]?.csrf).toBe(CSRF);
  expect(posts[0]?.body === "" || posts[0]?.body === "{}").toBe(true);
  expectNoSecrets(pageText());
});

test("pv-r22: архивный ученик возвращается", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/students/${STUDENT}/unarchive`) {
      return new Response(null, { status: 204 });
    }
    if (method === "GET" && path === `/students/${STUDENT}`) {
      return Response.json(studentBody("archived", "given"));
    }
    return happyReply(method, path);
  });
  renderCard();

  expect(
    await screen.findByRole("button", { name: /^вернуть$/i }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: /^архив$/i })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /^вернуть$/i }));
  await waitFor(async () => {
    const posts = (await callRecords(fetchMock)).filter(
      (record) =>
        record.method === "POST" &&
        record.url.includes(`/students/${STUDENT}/unarchive`),
    );
    expect(posts).toHaveLength(1);
    expect(posts[0]?.csrf).toBe(CSRF);
  });
});

test("wb-r6: архив, код forbidden", async () => {
  installFetch((method, path) => {
    if (method === "POST" && path === `/students/${STUDENT}/archive`) {
      return Response.json(errorBody("forbidden"), { status: 403 });
    }
    return happyReply(method, path);
  });
  renderCard();

  fireEvent.click(await screen.findByRole("button", { name: /^архив$/i }));
  expect(
    await screen.findByText("Нет доступа или объекта не существует"),
  ).toBeTruthy();
  const text = pageText();
  expect(text).not.toContain("forbidden");
  expect(text).not.toContain("goroutine");
  expectNoSecrets(text);
  expect(screen.queryByRole("button", { name: /^вернуть$/i })).toBeNull();
});

test("au-r8: «Приглашение» показывает ссылку", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/students/${STUDENT}/invites`) {
      return Response.json(link(INVITE_URL), { status: 201 });
    }
    return happyReply(method, path);
  });
  renderCard();

  expect(await screen.findByText(NAME)).toBeTruthy();
  expect(pageText()).not.toContain(INVITE_URL);
  fireEvent.click(screen.getByRole("button", { name: /^приглашение$/i }));
  expect(await screen.findByText(INVITE_URL)).toBeTruthy();
  expect(pageText()).toContain("15:00");
  expect(pageText()).not.toContain("12:00:00Z");
  const posts = (await callRecords(fetchMock)).filter(
    (record) =>
      record.method === "POST" &&
      record.url.includes(`/students/${STUDENT}/invites`),
  );
  expect(posts).toHaveLength(1);
  expect(posts[0]?.csrf).toBe(CSRF);
  expect(posts[0]?.body === "" || posts[0]?.body === "{}").toBe(true);
  expectNoSecrets(pageText());
});

test.each(INVITE_ERRORS)(
  "wb-r6: приглашение, код %s",
  async (code, status, sentence) => {
    installFetch((method, path) => {
      if (method === "POST" && path === `/students/${STUDENT}/invites`) {
        return Response.json(errorBody(code), { status });
      }
      return happyReply(method, path);
    });
    renderCard();

    fireEvent.click(
      await screen.findByRole("button", { name: /^приглашение$/i }),
    );
    expect(await screen.findByText(sentence)).toBeTruthy();
    const text = pageText();
    expect(text).not.toContain(code);
    expect(text).not.toContain("goroutine");
    expect(text).not.toContain(INVITE_URL);
    expectNoSecrets(text);
  },
);

test("pv-r12: «Ссылка согласия» для несовершеннолетнего — kind parent", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/students/${STUDENT}/consent-links`) {
      return Response.json(link(CONSENT_URL), { status: 201 });
    }
    return happyReply(method, path);
  });
  renderCard();

  expect(await screen.findByText(NAME)).toBeTruthy();
  expect(pageText()).not.toContain(CONSENT_URL);
  fireEvent.click(screen.getByRole("button", { name: /^ссылка согласия$/i }));
  expect(await screen.findByText(CONSENT_URL)).toBeTruthy();
  const posts = (await callRecords(fetchMock)).filter(
    (record) =>
      record.method === "POST" &&
      record.url.includes(`/students/${STUDENT}/consent-links`),
  );
  expect(posts).toHaveLength(1);
  expect(posts[0]?.csrf).toBe(CSRF);
  expect(JSON.parse(posts[0]?.body ?? "")).toEqual({ kind: "parent" });
  expectNoSecrets(pageText());
});

test("wb-r9: «отменить отзыв» только при отозванном согласии", async () => {
  const fetchMock = installFetch((method, path) => {
    if (
      method === "POST" &&
      path === `/students/${STUDENT}/consent/cancel-revoke`
    ) {
      return new Response(null, { status: 204 });
    }
    if (method === "GET" && path === `/students/${STUDENT}`) {
      return Response.json(studentBody("active", "revoked"));
    }
    return happyReply(method, path);
  });
  renderCard();

  fireEvent.click(
    await screen.findByRole("button", { name: /^отменить отзыв$/i }),
  );
  await waitFor(async () => {
    const posts = (await callRecords(fetchMock)).filter(
      (record) =>
        record.method === "POST" &&
        record.url.includes(`/students/${STUDENT}/consent/cancel-revoke`),
    );
    expect(posts).toHaveLength(1);
    expect(posts[0]?.csrf).toBe(CSRF);
    expect(posts[0]?.body === "" || posts[0]?.body === "{}").toBe(true);
  });
  expectNoSecrets(pageText());
});
