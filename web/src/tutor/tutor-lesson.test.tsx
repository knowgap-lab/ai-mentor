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
import { App } from "../App";
import { TutorLessonPage } from "./TutorLessonPage";

// Пакет W6, экран «Урок» (web.md, строка репетитора).
// Маршрут теста: /tutor/lessons/:lessonId. Ключ реестра — «Репетитор/Урок».
// web/screens.json этот файл не заполняет. Экран «Ученик/Урок» (/lesson/:lessonId) не трогаем.
// GET /lessons/{lesson} — LessonDetail (LS-R14).
// POST /lessons/{lesson}/move {starts_at} — Lesson (LS-R4).
// POST /lessons/{lesson}/cancel — PlanView (LS-R6).
// POST /lessons/{lesson}/skip — PlanView (PL-R4a).
// POST /lessons/{lesson}/marks — LessonDetail, тело Marks или {coverage} (LS-R16, LS-R20).
// Состав: этапы, блоки, практика, снимок, две отметки, перенос, отмена, пропуск.
// Перенесённое стоит раньше этапа снимка (LS-R14). Отметки можно ставить и по нему (LS-R19).
// «пропущен» — PL-R4a. «перенесено» и «добавлено» — только если признак истинный (PL-R8).
// Нет starts_at — фраза «неделя с понедельника» и planned_week (LS-R3).
// Есть starts_at — время Europe/Minsk, не сырой UTC (WB-R2).
// source l_file — «файл урока» (глоссарий, LS-R13) и один выбор «пройдено всё» /
// «не пройдено ничего» вместо двух отметок (LS-R20). Кнопка сразу шлёт coverage.
// source snapshot — две группы. Имена групп — определение «отметки урока» (D82):
// «последний объяснённый блок теории» и «последняя решённая задача практики».
// В группе радиокнопки: id блока или variant_id, плюс «нет» (LS-R16).
// Кнопка «две отметки» шлёт Marks. Практика уходит как {stage_id, position}.
// ТРАКТОВКА: подписи поля переноса в web.md нет. На экране ровно одно текстовое поле.
// Значение — местное время Минска ГГГГ-ММ-ДДTчч:мм. В starts_at уходит UTC с секундами и Z.
// ВОПРОС: русские слова для planned, frozen и held не заданы. Тест их не фиксирует.
// ВОПРОС: слова «загрузка» и «пусто» не заданы.
// ВОПРОС: текста для already_marked, lesson_not_held, slot_taken и mark_not_in_lesson
// в таблице web.md нет. Тест эти коды не шлёт.
// ВОПРОС: слово подтверждения переноса, отмены, пропуска и отметок не задано
// (WB-R31 перечисляет выдано, сдано, отозвано). Тест такое слово не требует.
// ВОПРОС: версия каталога есть в LS-R10, в LessonDetail её нет. Тест её не требует.
// ВОПРОС: как в jsdom проверять ширину 360 (WB-R1). Тест ширину не проверяет.
// Ключ, решение и «верная опция» в ответ подмешаны: на этом экране их нет (WB-R4).
// «показать» нет. GET /preparation, /conspect и /homework-draft этот экран не вызывает.

const CSRF = "csrf-w6";
const LESSON = "lesson-w6";
const TITLE = "Выдуманные доли";
const KIND = "лекция";
const TOPIC = "Выдуманная доля";
const STAGE_TITLE = "Доли на доске";
const WARMUP_TITLE = "Разминка";
const CARRIED = "block-w6-carried";
const THEORY = "block-w6-theory";
const FROM_LESSON = "lesson-w6-prev";
const STAGE_ID = "stage-w6";
const VARIANT = "var-w6";
const HIDDEN = "918273";
const SOLUTION = "ход-w6-секрет";
const MARK = "верная опция";
const WEEK_TITLE = "Выдуманная неделя";
const WEEK = "2026-W46";
const NEXT_LOCAL = "2026-11-20T21:30";
const NEXT_UTC = "2026-11-20T18:30:00Z";
const MOVED_UTC = "2026-11-21T10:00:00Z";
const CANCEL_UTC = "2026-12-01T16:00:00Z";
const THEORY_GROUP = /^последний объяснённый блок теории$/i;
const PRACTICE_GROUP = /^последняя решённая задача практики$/i;

const LOAD_ERRORS: [string, number, string][] = [
  ["forbidden", 403, "Нет доступа или объекта не существует"],
  ["unavailable", 503, "Нет связи, попробуйте ещё раз"],
  ["login_stale", 401, "Войдите ещё раз"],
];

const POST_ERRORS: [string, number, string][] = [
  ...LOAD_ERRORS,
  ["csrf", 403, "Обновите страницу и повторите"],
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

function lessonCore(title = TITLE) {
  return {
    id: LESSON,
    template_id: "tpl-w6",
    place_no: 14,
    title,
    kind: KIND,
    topics: [TOPIC],
    state: "frozen",
    starts_at: "2026-11-03T15:00:00Z",
    moved: true,
    added: false,
    marks: null,
    key: HIDDEN,
  };
}

function detailBody() {
  return {
    lesson: lessonCore(),
    carried_in: {
      blocks: [CARRIED],
      practice: [],
      from_lesson_id: FROM_LESSON,
    },
    stages: [
      {
        id: "stage-w6-warm",
        title: WARMUP_TITLE,
        kind: "разминка",
        minutes: [5, 40],
        blocks: [],
        practice: [],
      },
      {
        id: STAGE_ID,
        title: STAGE_TITLE,
        kind: "объяснение",
        minutes: [8, 21],
        blocks: [THEORY],
        practice: [
          {
            variant_id: VARIANT,
            type_id: "type-w6",
            position: 1,
            role: "practice",
          },
        ],
      },
    ],
    source: "snapshot",
    key: HIDDEN,
    show: { key: HIDDEN, solution_block: SOLUTION },
    options: [{ n: 1, text: "десять", mark: MARK }],
  };
}

function weekBody() {
  return {
    lesson: {
      id: LESSON,
      template_id: "tpl-w6",
      place_no: 15,
      title: WEEK_TITLE,
      kind: KIND,
      topics: [TOPIC],
      state: "planned",
      planned_week: WEEK,
      moved: false,
      added: true,
      marks: null,
    },
    stages: [
      {
        id: STAGE_ID,
        title: STAGE_TITLE,
        kind: "объяснение",
        minutes: [8, 21],
        blocks: [THEORY],
        practice: [],
      },
    ],
    source: "snapshot",
  };
}

function fileBody() {
  return {
    lesson: {
      ...lessonCore(),
      state: "held",
      marks: null,
    },
    stages: [],
    source: "l_file",
  };
}

function emptyBody() {
  return {
    lesson: lessonCore(),
    stages: [],
    source: "snapshot",
  };
}

function errorBody(code: string) {
  return { code, message: "goroutine 7 [running]", details: { key: HIDDEN } };
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
  if (method === "GET" && path === `/lessons/${LESSON}`) {
    return Response.json(detailBody());
  }
  return Response.json({ code: "forbidden" }, { status: 404 });
}

function renderLesson() {
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/tutor/lessons/${LESSON}`]}>
        <Routes>
          <Route
            path="/tutor/lessons/:lessonId"
            element={<TutorLessonPage />}
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
  expect(text).not.toContain(SOLUTION);
  expect(text).not.toContain(MARK);
  expect(text).not.toContain("верная");
  expect(screen.queryByRole("button", { name: /^показать$/i })).toBeNull();
}

function timeField(): HTMLElement {
  const skip = new Set([
    "radio",
    "checkbox",
    "hidden",
    "button",
    "submit",
    "file",
  ]);
  const fields = [...document.querySelectorAll("input, textarea")].filter(
    (node) => !skip.has((node as HTMLInputElement).type),
  );
  expect(fields).toHaveLength(1);
  return fields[0] as HTMLElement;
}

function markGroup(name: RegExp): HTMLElement {
  return (
    screen.queryByRole("radiogroup", { name }) ??
    screen.getByRole("group", { name })
  );
}

function chooseMark(groupName: RegExp, optionName: string | RegExp) {
  fireEvent.click(
    within(markGroup(groupName)).getByRole("radio", { name: optionName }),
  );
}

test("w6: маршрут урока в приложении", async () => {
  installFetch(happyReply);
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/tutor/lessons/${LESSON}`]}>
        <App />
      </MemoryRouter>
    </MantineProvider>,
  );
  expect(await screen.findByRole("heading", { name: /^Урок$/ })).toBeTruthy();
  expect(await screen.findByText(TITLE)).toBeTruthy();
});

test("w6: урок собирает состав и не показывает ключ", async () => {
  const fetchMock = installFetch(happyReply);
  renderLesson();

  expect(await screen.findByRole("heading", { name: /^Урок$/ })).toBeTruthy();
  expect(await screen.findByText(TITLE)).toBeTruthy();
  expect(screen.getByText(STAGE_TITLE)).toBeTruthy();
  expect(screen.getByText(WARMUP_TITLE)).toBeTruthy();

  const text = pageText();
  expect(text).toContain(KIND);
  expect(text).toContain(TOPIC);
  expect(text).toContain("14");
  expect(text).toContain(CARRIED);
  expect(text).toContain(THEORY);
  expect(text).toContain(FROM_LESSON);
  expect(text).toContain(VARIANT);
  expect(text).toMatch(/снимок/i);
  expect(text).toMatch(/две отметки/i);
  expect(text).toContain("перенесено");
  expect(text).not.toContain("добавлено");
  expect(text).not.toContain("пропущен");
  expect(text).toContain("18:00");
  expect(text).not.toContain("15:00:00Z");
  expect(text).toMatch(/5\D+40/);
  expect(text).toMatch(/8\D+21/);
  const carriedAt = text.indexOf(CARRIED);
  const stageAt = text.indexOf(STAGE_TITLE);
  expect(carriedAt).toBeGreaterThanOrEqual(0);
  expect(stageAt).toBeGreaterThan(carriedAt);
  expect(text).not.toContain("snapshot");
  expect(text).not.toContain("l_file");
  expect(text).not.toContain("файл урока");
  expectNoSecrets(text);
  expect(screen.getByRole("button", { name: /^перенос$/i })).toBeTruthy();
  expect(screen.getByRole("button", { name: /^отмена$/i })).toBeTruthy();
  expect(screen.getByRole("button", { name: /^пропуск$/i })).toBeTruthy();
  expect(screen.getByRole("button", { name: /^две отметки$/i })).toBeTruthy();
  expect(screen.queryByRole("button", { name: /^пройдено всё$/i })).toBeNull();
  expect(
    screen.queryByRole("button", { name: /^не пройдено ничего$/i }),
  ).toBeNull();
  expect(timeField()).toBeTruthy();
  expect(
    within(markGroup(THEORY_GROUP)).getByRole("radio", { name: CARRIED }),
  ).toBeTruthy();
  expect(
    within(markGroup(THEORY_GROUP)).getByRole("radio", { name: THEORY }),
  ).toBeTruthy();
  expect(
    within(markGroup(THEORY_GROUP)).getByRole("radio", { name: /^нет$/i }),
  ).toBeTruthy();
  expect(
    within(markGroup(PRACTICE_GROUP)).getByRole("radio", { name: VARIANT }),
  ).toBeTruthy();
  expect(
    within(markGroup(PRACTICE_GROUP)).getByRole("radio", { name: /^нет$/i }),
  ).toBeTruthy();

  const records = await callRecords(fetchMock);
  expect(
    records.some((record) => {
      if (record.method !== "GET") {
        return false;
      }
      const url = new URL(record.url, "http://localhost");
      return url.pathname.replace(/\/$/, "") === `/lessons/${LESSON}`;
    }),
  ).toBe(true);
  expect(records.some((record) => record.url.includes("/preparation"))).toBe(
    false,
  );
  expect(records.some((record) => record.url.includes("/conspect"))).toBe(
    false,
  );
  expect(records.some((record) => record.url.includes("/homework-draft"))).toBe(
    false,
  );
});

test("w6: пустой снимок без ключа и без чужих кнопок", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}`) {
      return Response.json(emptyBody());
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderLesson();

  expect(await screen.findByRole("heading", { name: /^Урок$/ })).toBeTruthy();
  expect(await screen.findByText(TITLE)).toBeTruthy();
  const text = pageText();
  expect(text).toMatch(/снимок/i);
  expect(text).not.toContain(STAGE_TITLE);
  expect(text).not.toContain(VARIANT);
  expect(text).not.toContain("файл урока");
  expect(screen.getByRole("button", { name: /^перенос$/i })).toBeTruthy();
  expect(screen.getByRole("button", { name: /^отмена$/i })).toBeTruthy();
  expect(screen.getByRole("button", { name: /^пропуск$/i })).toBeTruthy();
  expect(timeField()).toBeTruthy();
  expectNoSecrets(text);
});

test("ls-r3: без занятия — неделя с понедельника", async () => {
  installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}`) {
      return Response.json(weekBody());
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderLesson();

  expect(await screen.findByText(WEEK_TITLE)).toBeTruthy();
  const text = pageText();
  expect(text).toContain("неделя с понедельника");
  expect(text).toContain(WEEK);
  expect(text).toContain("добавлено");
  expect(text).not.toContain("перенесено");
  expect(text).not.toContain("15:00:00Z");
});

test("ls-r13: урок из файла — один выбор вместо двух отметок", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/lessons/${LESSON}/marks`) {
      return Response.json(fileBody());
    }
    if (method === "GET" && path === `/lessons/${LESSON}`) {
      return Response.json(fileBody());
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderLesson();

  expect(await screen.findByText(TITLE)).toBeTruthy();
  const text = pageText();
  expect(text).toMatch(/файл урока/i);
  expect(text).not.toContain("l_file");
  expect(text).not.toContain("snapshot");
  expect(screen.queryByRole("radiogroup", { name: THEORY_GROUP })).toBeNull();
  expect(screen.queryByRole("group", { name: THEORY_GROUP })).toBeNull();
  expect(screen.queryByRole("radiogroup", { name: PRACTICE_GROUP })).toBeNull();
  expect(screen.queryByRole("group", { name: PRACTICE_GROUP })).toBeNull();
  expect(screen.getByRole("button", { name: /^перенос$/i })).toBeTruthy();
  expect(screen.getByRole("button", { name: /^отмена$/i })).toBeTruthy();
  expect(screen.getByRole("button", { name: /^пропуск$/i })).toBeTruthy();
  expect(timeField()).toBeTruthy();
  expectNoSecrets(text);

  fireEvent.click(screen.getByRole("button", { name: /^пройдено всё$/i }));
  await waitFor(async () => {
    const posts = (await callRecords(fetchMock)).filter(
      (record) =>
        record.method === "POST" &&
        record.url.includes(`/lessons/${LESSON}/marks`),
    );
    expect(posts).toHaveLength(1);
    expect(posts[0]?.csrf).toBe(CSRF);
    expect(JSON.parse(posts[0]?.body ?? "")).toEqual({ coverage: "all" });
  });
});

test("ls-r20: «не пройдено ничего» шлёт coverage none", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/lessons/${LESSON}/marks`) {
      return Response.json(fileBody());
    }
    if (method === "GET" && path === `/lessons/${LESSON}`) {
      return Response.json(fileBody());
    }
    return Response.json({ code: "forbidden" }, { status: 404 });
  });
  renderLesson();

  fireEvent.click(
    await screen.findByRole("button", { name: /^не пройдено ничего$/i }),
  );
  await waitFor(async () => {
    const posts = (await callRecords(fetchMock)).filter(
      (record) =>
        record.method === "POST" &&
        record.url.includes(`/lessons/${LESSON}/marks`),
    );
    expect(posts).toHaveLength(1);
    expect(posts[0]?.csrf).toBe(CSRF);
    expect(JSON.parse(posts[0]?.body ?? "")).toEqual({ coverage: "none" });
  });
});

test("wb-r30: урок без сети — «Нет связи» и «Повторить»", async () => {
  let online = false;
  const fetchMock = installFetch((method, path) => {
    if (method === "GET" && path === `/lessons/${LESSON}`) {
      if (!online) {
        return "network";
      }
      return Response.json(detailBody());
    }
    return "network";
  });
  renderLesson();

  expect(await screen.findByText("Нет связи")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Повторить" })).toBeTruthy();
  expect(pageText()).not.toContain("попробуйте ещё раз");
  expect(pageText()).not.toContain("Failed to fetch");
  expect(pageText()).not.toContain("TypeError");
  expectNoSecrets(pageText());

  online = true;
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(await screen.findByText(TITLE)).toBeTruthy();
  const gets = (await callRecords(fetchMock)).filter((record) => {
    if (record.method !== "GET") {
      return false;
    }
    const url = new URL(record.url, "http://localhost");
    return url.pathname.replace(/\/$/, "") === `/lessons/${LESSON}`;
  });
  expect(gets.length).toBeGreaterThanOrEqual(2);
});

test.each(LOAD_ERRORS)(
  "wb-r6: загрузка урока, код %s",
  async (code, status, sentence) => {
    installFetch((method, path) => {
      if (method === "GET" && path === `/lessons/${LESSON}`) {
        return Response.json(errorBody(code), { status });
      }
      return Response.json({ code: "forbidden" }, { status: 404 });
    });
    renderLesson();

    expect(await screen.findByText(sentence)).toBeTruthy();
    const text = pageText();
    expect(text).not.toContain(code);
    expect(text).not.toContain("goroutine");
    expectNoSecrets(text);
  },
);

test("ls-r4: «перенос» шлёт местное время как UTC", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/lessons/${LESSON}/move`) {
      return Response.json({
        ...lessonCore(),
        starts_at: MOVED_UTC,
        moved: true,
      });
    }
    return happyReply(method, path);
  });
  renderLesson();

  expect(await screen.findByText(TITLE)).toBeTruthy();
  fireEvent.change(timeField(), { target: { value: NEXT_LOCAL } });
  fireEvent.click(screen.getByRole("button", { name: /^перенос$/i }));
  expect(await screen.findByText("13:00")).toBeTruthy();
  expect(pageText()).not.toContain("10:00:00Z");
  const posts = (await callRecords(fetchMock)).filter(
    (record) =>
      record.method === "POST" &&
      record.url.includes(`/lessons/${LESSON}/move`),
  );
  expect(posts).toHaveLength(1);
  expect(posts[0]?.csrf).toBe(CSRF);
  expect(JSON.parse(posts[0]?.body ?? "")).toEqual({ starts_at: NEXT_UTC });
  expectNoSecrets(pageText());
});

test.each(POST_ERRORS)(
  "wb-r6: перенос, код %s",
  async (code, status, sentence) => {
    installFetch((method, path) => {
      if (method === "POST" && path === `/lessons/${LESSON}/move`) {
        return Response.json(errorBody(code), { status });
      }
      return happyReply(method, path);
    });
    renderLesson();

    expect(await screen.findByText(TITLE)).toBeTruthy();
    fireEvent.change(timeField(), { target: { value: NEXT_LOCAL } });
    fireEvent.click(screen.getByRole("button", { name: /^перенос$/i }));
    expect(await screen.findByText(sentence)).toBeTruthy();
    const text = pageText();
    expect(text).not.toContain(code);
    expect(text).not.toContain("goroutine");
    expect(text).not.toContain("13:00");
    expectNoSecrets(text);
  },
);

test("ls-r6: «отмена» показывает время из ответа", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/lessons/${LESSON}/cancel`) {
      return Response.json({
        lessons: [{ ...lessonCore(), starts_at: CANCEL_UTC, moved: true }],
        warnings: [],
      });
    }
    return happyReply(method, path);
  });
  renderLesson();

  fireEvent.click(await screen.findByRole("button", { name: /^отмена$/i }));
  expect(await screen.findByText("19:00")).toBeTruthy();
  expect(pageText()).not.toContain("16:00:00Z");
  const posts = (await callRecords(fetchMock)).filter(
    (record) =>
      record.method === "POST" &&
      record.url.includes(`/lessons/${LESSON}/cancel`),
  );
  expect(posts).toHaveLength(1);
  expect(posts[0]?.csrf).toBe(CSRF);
  expect(posts[0]?.body === "" || posts[0]?.body === "{}").toBe(true);
  expectNoSecrets(pageText());
});

test("pl-r4a: «пропуск» помечает урок пропущенным", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/lessons/${LESSON}/skip`) {
      return Response.json({
        lessons: [{ ...lessonCore(), state: "skipped", moved: false }],
        warnings: [],
      });
    }
    return happyReply(method, path);
  });
  renderLesson();

  expect(await screen.findByText(TITLE)).toBeTruthy();
  expect(pageText()).not.toContain("пропущен");
  fireEvent.click(screen.getByRole("button", { name: /^пропуск$/i }));
  expect(await screen.findByText("пропущен")).toBeTruthy();
  const posts = (await callRecords(fetchMock)).filter(
    (record) =>
      record.method === "POST" &&
      record.url.includes(`/lessons/${LESSON}/skip`),
  );
  expect(posts).toHaveLength(1);
  expect(posts[0]?.csrf).toBe(CSRF);
  expect(posts[0]?.body === "" || posts[0]?.body === "{}").toBe(true);
  expectNoSecrets(pageText());
});

test.each(POST_ERRORS)(
  "wb-r6: пропуск, код %s",
  async (code, status, sentence) => {
    installFetch((method, path) => {
      if (method === "POST" && path === `/lessons/${LESSON}/skip`) {
        return Response.json(errorBody(code), { status });
      }
      return happyReply(method, path);
    });
    renderLesson();

    fireEvent.click(await screen.findByRole("button", { name: /^пропуск$/i }));
    expect(await screen.findByText(sentence)).toBeTruthy();
    const text = pageText();
    expect(text).not.toContain("пропущен");
    expect(text).not.toContain(code);
    expect(text).not.toContain("goroutine");
    expectNoSecrets(text);
  },
);

test("ls-r16: «две отметки» шлют блок и задачу", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/lessons/${LESSON}/marks`) {
      return Response.json({
        ...detailBody(),
        lesson: {
          ...lessonCore(),
          state: "held",
          marks: {
            last_theory_block: CARRIED,
            last_practice: { stage_id: STAGE_ID, position: 1 },
          },
        },
      });
    }
    return happyReply(method, path);
  });
  renderLesson();

  expect(await screen.findByText(TITLE)).toBeTruthy();
  chooseMark(THEORY_GROUP, CARRIED);
  chooseMark(PRACTICE_GROUP, VARIANT);
  fireEvent.click(screen.getByRole("button", { name: /^две отметки$/i }));
  await waitFor(async () => {
    const posts = (await callRecords(fetchMock)).filter(
      (record) =>
        record.method === "POST" &&
        record.url.includes(`/lessons/${LESSON}/marks`),
    );
    expect(posts).toHaveLength(1);
    expect(posts[0]?.csrf).toBe(CSRF);
    expect(JSON.parse(posts[0]?.body ?? "")).toEqual({
      last_theory_block: CARRIED,
      last_practice: { stage_id: STAGE_ID, position: 1 },
    });
  });
  expectNoSecrets(pageText());
});

test("ls-r17: обе отметки «нет»", async () => {
  const fetchMock = installFetch((method, path) => {
    if (method === "POST" && path === `/lessons/${LESSON}/marks`) {
      return Response.json(detailBody());
    }
    return happyReply(method, path);
  });
  renderLesson();

  expect(await screen.findByText(TITLE)).toBeTruthy();
  chooseMark(THEORY_GROUP, /^нет$/i);
  chooseMark(PRACTICE_GROUP, /^нет$/i);
  fireEvent.click(screen.getByRole("button", { name: /^две отметки$/i }));
  await waitFor(async () => {
    const posts = (await callRecords(fetchMock)).filter(
      (record) =>
        record.method === "POST" &&
        record.url.includes(`/lessons/${LESSON}/marks`),
    );
    expect(posts).toHaveLength(1);
    expect(posts[0]?.csrf).toBe(CSRF);
    expect(JSON.parse(posts[0]?.body ?? "")).toEqual({
      last_theory_block: null,
      last_practice: null,
    });
  });
});

test("wb-r6: отметки, код forbidden", async () => {
  installFetch((method, path) => {
    if (method === "POST" && path === `/lessons/${LESSON}/marks`) {
      return Response.json(errorBody("forbidden"), { status: 403 });
    }
    return happyReply(method, path);
  });
  renderLesson();

  expect(await screen.findByText(TITLE)).toBeTruthy();
  chooseMark(THEORY_GROUP, /^нет$/i);
  chooseMark(PRACTICE_GROUP, /^нет$/i);
  fireEvent.click(screen.getByRole("button", { name: /^две отметки$/i }));
  expect(
    await screen.findByText("Нет доступа или объекта не существует"),
  ).toBeTruthy();
  const text = pageText();
  expect(text).not.toContain("forbidden");
  expect(text).not.toContain("goroutine");
  expectNoSecrets(text);
});
