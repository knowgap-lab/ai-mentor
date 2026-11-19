import { MantineProvider } from "@mantine/core";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { App } from "../App";

// Пакет W5 дополняет уже существующий экран «Ученики» (web.md, WB-R21, WB-R25).
// Замороженный TutorPage.test.tsx не меняется: клик по имени по-прежнему показывает
// сырой ввод на этом же экране. Этот файл клик не нажимает.
// GET /students — StudentSummary. Фразы показателей — из строки «Ученики»:
// «следующий урок», «ДЗ ждёт просмотра», «сдано», «просрочено»,
// «открытые споры», «не доставлено».
// Время next_lesson_at — Europe/Minsk (WB-R2).
// Плашка предупреждения — текст Warning.text, не английский код типа (WB-R25, NT-R17).
// «не доставлено» пишется, когда not_delivered истинно (NT-R10).
// ВОПРОС: писать ли нулевые показатели. Тест отсутствие нулей не требует.
// ВОПРОС: слова «загрузка» и «пусто» не заданы.
// ВОПРОС: как в jsdom проверять ширину 360 (WB-R1). Тест ширину не проверяет.

const NEAR = "Ольха";
const FAR = "Ель";
const TELEGRAM_ID = "tg-demo-tutor";
const WARN = "У Ольха нет новых вариантов типа Выдуманный тип";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
});

function summary(id: string, name: string, near: boolean) {
  if (!near) {
    return {
      id,
      display_name: name,
      next_lesson_at: "2026-11-20T15:30:00Z",
      homework_waiting_review: 0,
      homework_submitted: 0,
      homework_overdue: 0,
      open_disputes: 0,
      not_delivered: false,
      warnings: [],
    };
  }
  return {
    id,
    display_name: name,
    next_lesson_at: "2026-11-03T08:00:00Z",
    homework_waiting_review: 2,
    homework_submitted: 5,
    homework_overdue: 7,
    open_disputes: 4,
    not_delivered: true,
    warnings: [{ type: "variants_needed", text: WARN }],
  };
}

function installFetch(students: "list" | "empty" | "network") {
  let attempts = 0;
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
      if (method === "POST" && path.endsWith("/auth/dev")) {
        return Response.json({ user_id: "tutor-demo", role: "tutor" });
      }
      if (method === "GET" && path.endsWith("/students")) {
        attempts += 1;
        if (students === "network" && attempts === 1) {
          throw new TypeError("Failed to fetch");
        }
        if (students === "empty") {
          return Response.json([]);
        }
        return Response.json([
          summary("stu-far", FAR, false),
          summary("stu-near", NEAR, true),
        ]);
      }
      return Response.json([]);
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

function pageText() {
  for (const node of document.body.querySelectorAll("style")) {
    node.textContent = "";
  }
  return document.body.textContent ?? "";
}

function tokens(text: string): string[] {
  return text.split(/[^\p{L}\p{N}:]+/u).filter((part) => part !== "");
}

async function login() {
  fireEvent.change(screen.getByRole("textbox"), {
    target: { value: TELEGRAM_ID },
  });
  fireEvent.click(screen.getByRole("button", { name: "Войти" }));
  await screen.findByRole("list", { name: "Ученики" });
}

function row(name: string): HTMLElement {
  const list = screen.getByRole("list", { name: "Ученики" });
  const item = within(list)
    .getAllByRole("listitem")
    .find((candidate) => candidate.textContent?.includes(name));
  if (!item) {
    throw new Error(`нет строки ${name}`);
  }
  return item;
}

test("w5: у списка учеников есть показатели и плашка", async () => {
  installFetch("list");
  renderTutor();
  expect(await screen.findByRole("button", { name: "Войти" })).toBeTruthy();
  await login();

  const list = screen.getByRole("list", { name: "Ученики" });
  const items = within(list).getAllByRole("listitem");
  expect(items[0]?.textContent).toContain(NEAR);
  expect(items[1]?.textContent).toContain(FAR);

  const near = row(NEAR);
  const far = row(FAR);
  const nearText = near.textContent ?? "";
  const farText = far.textContent ?? "";
  const page = pageText();

  expect(page).toContain("следующий урок");
  expect(nearText).toContain("11:00");
  expect(farText).toContain("18:30");
  expect(nearText).not.toContain("08:00:00Z");
  expect(farText).not.toContain("15:30:00Z");
  expect(nearText).toContain("ДЗ ждёт просмотра");
  expect(nearText).toContain("сдано");
  expect(nearText).toContain("просрочено");
  expect(nearText).toContain("открытые споры");
  expect(nearText).toContain("не доставлено");
  expect(tokens(nearText)).toEqual(
    expect.arrayContaining(["2", "5", "7", "4"]),
  );
  expect(farText).not.toContain("не доставлено");
  expect(page).toContain(WARN);
  expect(page.indexOf(WARN)).toBeLessThan(page.indexOf(FAR));
  expect(page).not.toContain("variants_needed");
});

test("wb-r30: список учеников без сети — «Нет связи» и «Повторить»", async () => {
  const fetchMock = installFetch("network");
  renderTutor();
  fireEvent.change(await screen.findByRole("textbox"), {
    target: { value: TELEGRAM_ID },
  });
  fireEvent.click(screen.getByRole("button", { name: "Войти" }));

  expect(await screen.findByText("Нет связи")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Повторить" })).toBeTruthy();
  expect(pageText()).not.toContain("Failed to fetch");
  expect(pageText()).not.toContain("TypeError");

  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  await waitFor(() => {
    expect(screen.getByText(NEAR)).toBeTruthy();
  });
  const gets = fetchMock.mock.calls.filter((call) => {
    const [input, init] = call as [RequestInfo | URL, RequestInit | undefined];
    const url = input instanceof Request ? input.url : String(input);
    const method = (
      init?.method ?? (input instanceof Request ? input.method : "GET")
    ).toUpperCase();
    return method === "GET" && url.includes("/students");
  });
  expect(gets.length).toBeGreaterThanOrEqual(2);
});

test("w5: пустой список учеников остаётся экраном «Ученики»", async () => {
  installFetch("empty");
  renderTutor();
  fireEvent.change(await screen.findByRole("textbox"), {
    target: { value: TELEGRAM_ID },
  });
  fireEvent.click(screen.getByRole("button", { name: "Войти" }));

  expect(await screen.findByRole("heading", { name: "Ученики" })).toBeTruthy();
  expect(screen.queryByText(NEAR)).toBeNull();
  expect(screen.queryByText(FAR)).toBeNull();
});
