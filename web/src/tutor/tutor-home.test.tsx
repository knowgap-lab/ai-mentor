import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { MantineProvider } from "@mantine/core";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, expect, test } from "vitest";
import { theme } from "../design/theme";
import { TutorHomePage } from "./TutorPage";

const htmlPath = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../public/th.html",
);

afterEach(() => {
  cleanup();
});

test("tutor home: маршрут открывает статическую демо-страницу", () => {
  render(
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
  expect(screen.getByTitle("Дом репет").getAttribute("src")).toBe("/th.html");
});

test("tutor home: html содержит синтетических учеников и профиль", () => {
  const html = readFileSync(htmlPath, "utf8");
  expect(html).toContain("Марина Волкова");
  expect(html).toContain("Алина К.");
  expect(html).toContain("Telegram");
  expect(html).toContain("Переключить светлую и тёмную тему");
  expect(html).toContain("Синтетические ученики");
});
