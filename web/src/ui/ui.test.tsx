import { MantineProvider } from "@mantine/core";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import { theme } from "../design/theme";
import {
  ActionBar,
  List,
  ListRow,
  STATUSES,
  StatusBadge,
  type Status,
} from ".";

function wrap(node: ReactNode) {
  return render(
    <MantineProvider theme={theme}>
      <MemoryRouter>{node}</MemoryRouter>
    </MantineProvider>,
  );
}

describe("StatusBadge (DS-R3)", () => {
  const expected: Record<Status, string> = {
    not_answered: "не отвечено",
    correct: "верно",
    partial: "частично",
    wrong: "неверно",
    closed: "закрыто",
    submitted: "сдано",
    awaiting_review: "ждёт просмотра",
    overdue: "просрочено",
  };

  it("у каждого состояния своя подпись", () => {
    for (const state of Object.keys(STATUSES) as Status[]) {
      const { unmount } = wrap(<StatusBadge state={state} />);
      expect(screen.getByText(expected[state])).toBeTruthy();
      unmount();
    }
  });

  it("неверно — красная ручка", () => {
    expect(STATUSES.wrong.color).toBe("red");
    expect(STATUSES.correct.color).toBe("green");
  });
});

describe("ListRow (DS-R11)", () => {
  it("вся строка — одна ссылка", () => {
    wrap(
      <List label="Выдачи">
        <ListRow
          to="/assignment/a1"
          title="ДЗ к уроку 12"
          subtitle="срок 14 ноября"
        />
      </List>,
    );
    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toBe("/assignment/a1");
    expect(link.textContent).toContain("ДЗ к уроку 12");
    expect(link.textContent).toContain("срок 14 ноября");
  });
});

describe("ActionBar (DS-R10)", () => {
  it("главное и второстепенное действие", () => {
    wrap(
      <ActionBar
        primary={{ label: "Сдать", onClick: () => undefined }}
        secondary={{ label: "К списку", onClick: () => undefined }}
      />,
    );
    expect(screen.getByRole("button", { name: "Сдать" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "К списку" })).toBeTruthy();
  });
});
