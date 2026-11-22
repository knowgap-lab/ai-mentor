import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { List, ListButton, StatusBadge } from ".";

const meta = {
  title: "UI/ListButton",
  component: ListButton,
  tags: ["autodocs"],
} satisfies Meta<typeof ListButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const TitleOnly: Story = {
  render: (args) => (
    <List label="Действия">
      <ListButton {...args} />
    </List>
  ),
  args: {
    title: "Выйти из сеанса",
    onClick: fn(),
  },
};

export const WithSubtitle: Story = {
  render: (args) => (
    <List label="Действия">
      <ListButton {...args} />
    </List>
  ),
  args: {
    title: "Удалить выдачу",
    subtitle: "Только черновик",
    onClick: fn(),
  },
};

export const WithAside: Story = {
  render: (args) => (
    <List label="Действия">
      <ListButton {...args} />
    </List>
  ),
  args: {
    title: "Открыть выдачу",
    subtitle: "ДЗ к уроку 11",
    aside: <StatusBadge state="awaiting_review" />,
    onClick: fn(),
  },
};