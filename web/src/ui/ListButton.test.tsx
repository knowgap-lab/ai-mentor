import { MantineProvider } from "@mantine/core";
import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { theme } from "../design/theme";
import { ChoiceRow, List, ListButton } from ".";

function wrap(node: ReactNode) {
  return render(
    <MantineProvider theme={theme}>
      <MemoryRouter>{node}</MemoryRouter>
    </MantineProvider>,
  );
}

describe("ListButton (DS-R11)", () => {
  it("вся строка — одна кнопка", () => {
    const onClick = vi.fn();
    wrap(
      <List label="Ученики">
        <ListButton title="Лиса" subtitle="урок 3 ноября" onClick={onClick} />
      </List>,
    );
    const button = screen.getByRole("button", { name: /Лиса/ });
    expect(screen.getAllByRole("button")).toHaveLength(1);
    expect(button.textContent).toContain("Лиса");
    expect(button.textContent).toContain("урок 3 ноября");
    fireEvent.click(screen.getByText("Лиса"));
    expect(onClick).toHaveBeenCalledOnce();
  });
});

describe("ChoiceRow (DS-R8)", () => {
  it("держит вариант ответа целиком", () => {
    wrap(
      <ChoiceRow>
        <span>вариант 3</span>
      </ChoiceRow>,
    );
    const option = screen.getByText("вариант 3");
    expect(option.parentElement?.tagName).toBe("DIV");
    expect(option.parentElement?.className).not.toBe("");
  });
});
