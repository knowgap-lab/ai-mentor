import { MantineProvider } from "@mantine/core";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, expect, test } from "vitest";
import { theme } from "../design/theme";
import { TutorHomePage } from "./TutorHomePage";
import { TUTOR_HOME_PROFILE, TUTOR_HOME_STUDENTS } from "./tutor-home-data";

afterEach(() => {
  cleanup();
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

test("tutor home: синтетические ученики, профиль и прогресс", () => {
  renderHome();

  expect(
    screen.getByRole("heading", { name: "Домашняя репетитора" }),
  ).toBeTruthy();
  expect(screen.getByText(TUTOR_HOME_PROFILE.displayName)).toBeTruthy();
  expect(screen.getByText(TUTOR_HOME_PROFILE.about)).toBeTruthy();

  for (const student of TUTOR_HOME_STUDENTS) {
    expect(screen.getByText(student.displayName)).toBeTruthy();
    expect(
      screen.getByText(`${student.homeworkDone} из ${student.homeworkTotal}`),
    ).toBeTruthy();
    expect(screen.getByText(`${student.coursePercent}%`)).toBeTruthy();
  }

  expect(screen.getByRole("link", { name: "Telegram" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "VK" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "Сайт" })).toBeTruthy();
  const cabinet = screen.getByRole("link", {
    name: "Войти в кабинет репетитора",
  });
  expect(cabinet.getAttribute("href")).toBe("/tutor");
});

test("tutor home: переключатель темы меняет data-am-scheme", () => {
  renderHome();

  const root = screen.getByTestId("tutor-home-root");
  expect(root.getAttribute("data-am-scheme")).toBe("light");

  const toggle = screen.getByRole("switch", {
    name: "Переключить светлую и тёмную тему",
  });
  fireEvent.click(toggle);

  expect(root.getAttribute("data-am-scheme")).toBe("dark");
  expect(window.localStorage.getItem("am-tutor-home-scheme")).toBe("dark");

  fireEvent.click(toggle);
  expect(root.getAttribute("data-am-scheme")).toBe("light");
});

test("tutor home: адаптивная сетка учеников", () => {
  renderHome();
  const board = screen.getByRole("list", { name: "Синтетические ученики" });
  expect(board.className).toMatch(/board/);
  expect(board.querySelectorAll("li").length).toBe(TUTOR_HOME_STUDENTS.length);
});
