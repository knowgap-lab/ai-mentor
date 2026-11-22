import { TextInput } from "@mantine/core";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChoiceRow } from ".";

const meta = {
  title: "UI/ChoiceRow",
  component: ChoiceRow,
  tags: ["autodocs"],
} satisfies Meta<typeof ChoiceRow>;

export default meta;
type Story = StoryObj<typeof meta>;

export const TextField: Story = {
  args: {
    children: <TextInput label="Ответ" placeholder="целое число" />,
  },
};

export const PlainContent: Story = {
  args: {
    children: <span>Вариант A</span>,
  },
};
