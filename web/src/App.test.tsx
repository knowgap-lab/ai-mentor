import { MantineProvider } from "@mantine/core";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { expect, test } from "vitest";
import { App } from "./App";

test("страница показывает название", () => {
  render(
    <MantineProvider>
      <MemoryRouter>
        <App />
      </MemoryRouter>
    </MantineProvider>,
  );
  expect(screen.getByText("ИИ-ментор")).toBeTruthy();
  expect(screen.getByRole("link", { name: "дальше" })).toBeTruthy();
});
