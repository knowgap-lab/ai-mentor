import type { Meta, StoryObj } from "@storybook/react-vite";
import { ActionBar, List, ListRow, Screen, StatusBadge } from ".";

const meta = {
  title: "UI/Screen",
  component: Screen,
  tags: ["autodocs"],
} satisfies Meta<typeof Screen>;

export default meta;
type Story = StoryObj<typeof meta>;

export const TitleOnly: Story = {
  args: {
    title: "Выдачи",
    children: (
      <List label="Выдачи">
        <ListRow to="/assignments/a1" title="ДЗ к уроку 12" />
      </List>
    ),
  },
};

export const WithBackLink: Story = {
  args: {
    title: "ДЗ к уроку 12",
    backTo: "/assignments",
    children: <p>Текст экрана.</p>,
  },
};

export const WithActionBar: Story = {
  args: {
    title: "Задача 4",
    backTo: "/assignment/a1",
    withActionBar: true,
    children: (
      <ActionBar
        primary={{ label: "Сдать", onClick: () => undefined }}
        secondary={{ label: "К списку", onClick: () => undefined }}
      />
    ),
  },
};

export const WithAsideInList: Story = {
  args: {
    title: "Выдачи",
    children: (
      <List label="Выдачи">
        <ListRow
          to="/assignments/a1"
          title="ДЗ к уроку 12"
          subtitle="срок 14 декабря"
          aside={<StatusBadge state="not_answered" />}
        />
      </List>
    ),
  },
};
