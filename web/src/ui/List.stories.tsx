import type { Meta, StoryObj } from "@storybook/react-vite";
import { List, ListRow, StatusBadge } from ".";

const meta = {
  title: "UI/List",
  component: List,
  tags: ["autodocs"],
} satisfies Meta<typeof List>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SingleRow: Story = {
  args: {
    label: "Выдачи",
    children: <ListRow to="/assignments/a1" title="ДЗ к уроку 12" />,
  },
};

export const WithSubtitle: Story = {
  args: {
    label: "Выдачи",
    children: (
      <ListRow
        to="/assignments/a1"
        title="ДЗ к уроку 12"
        subtitle="Математика · срок 14 декабря"
      />
    ),
  },
};

export const WithAside: Story = {
  args: {
    label: "Выдачи",
    children: (
      <ListRow
        to="/assignments/a1"
        title="ДЗ к уроку 12"
        subtitle="3 из 8"
        aside={<StatusBadge state="submitted" />}
      />
    ),
  },
};

export const MultipleRows: Story = {
  args: {
    label: "Выдачи",
    children: (
      <>
        <ListRow
          to="/assignments/a1"
          title="ДЗ к уроку 12"
          subtitle="не отвечено"
          aside={<StatusBadge state="not_answered" />}
        />
        <ListRow
          to="/assignments/a2"
          title="Срез по теме"
          subtitle="просрочено"
          aside={<StatusBadge state="overdue" />}
        />
      </>
    ),
  },
};