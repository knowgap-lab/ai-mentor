import { Text } from "@mantine/core";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { StatusBadge, TaskSheet } from ".";

const meta = {
  title: "UI/TaskSheet",
  component: TaskSheet,
  tags: ["autodocs"],
} satisfies Meta<typeof TaskSheet>;

export default meta;
type Story = StoryObj<typeof meta>;

export const WithLabel: Story = {
  args: {
    label: "Задача 4 из 8 · часть В",
    children: (
      <Text>Найдите наименьшее целое решение неравенства 2x − 3 ≥ 5.</Text>
    ),
  },
};

export const WithoutLabel: Story = {
  args: {
    children: <Text>Условие задачи без отдельной подписи сверху.</Text>,
  },
};

export const WithFeedback: Story = {
  args: {
    label: "Задача 2 из 8",
    children: (
      <>
        <Text>Сколько корней у уравнения x² = 4?</Text>
        <StatusBadge state="wrong" />
      </>
    ),
  },
};