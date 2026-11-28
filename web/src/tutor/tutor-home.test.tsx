import { MantineProvider } from "@mantine/core";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { theme } from "../design/theme";
import { TutorHomePage } from "./TutorPage";
import pack from "../../public/th.json";

const STUDENTS = [
  { name: "Алина К.", done: 6, total: 8, course: 58 },
  { name: "Богдан М.", done: 3, total: 5, course: 41 },
  { name: "Вера С.", done: 8, total: 8, course: 72 },
  { name: "Глеб Т.", done: 1, total: 6, course: 22 },
  { name: "Дина Л.", done: 4, total: 7, course: 49 },
  { name: "Егор П.", done: 5, total: 5, course: 63 },
] as const;

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => Promise.resolve(Response.json(pack))),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

function renderHome() {
  return render(
    <MantineProvider
      theme={theme}
      forceColorScheme="light"
      withCssVariables={false}
      withGlobalClasses={false}
    >
      <MemoryRouter initialEntries={["/tutor/home"]}>
        <Routes>
          <Route path="/tutor/home" element={<TutorHomePage />} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );
}

test("tutor home: рендерит доску, профиль и соцссылки", async () => {
  renderHome();

  expect(
    await screen.findByRole("heading", { name: "Домашняя репетитора" }),
  ).toBeTruthy();
  expect(screen.getByText("Марина Волкова")).toBeTruthy();
  expect(screen.getByText(/разбор ошибок/i)).toBeTruthy();

  for (const student of STUDENTS) {
    expect(screen.getByText(student.name, { exact: false })).toBeTruthy();
    expect(
      screen.getByText(
        `ДЗ ${student.done} из ${student.total} · курс ${student.course}%`,
      ),
    ).toBeTruthy();
  }

  expect(screen.getByRole("link", { name: "Telegram" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "VK" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "Сайт" })).toBeTruthy();
  expect(
    screen.getByRole("link", { name: "Войти в кабинет репетитора" }),
  ).toBeTruthy();
});

test("tutor home: переключатель темы", async () => {
  renderHome();
  const root = await screen.findByTestId("th");
  expect(root.getAttribute("data-am-scheme")).toBe("light");
  fireEvent.click(
    screen.getByRole("checkbox", {
      name: "Переключить светлую и тёмную тему",
    }),
  );
  expect(root.getAttribute("data-am-scheme")).toBe("dark");
});

test("tutor home: сетка из шести учеников", async () => {
  renderHome();
  const board = await screen.findByRole("list", { name: "Синтетические ученики" });
  expect(board.querySelectorAll("li").length).toBe(STUDENTS.length);
});