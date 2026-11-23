import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ActionBar } from ".";

const meta = {
  title: "UI/ActionBar",
  component: ActionBar,
  tags: ["autodocs"],
} satisfies Meta<typeof ActionBar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const PrimaryOnly: Story = {
  args: {
    primary: { label: "Сдать", onClick: fn() },
  },
};

export const PrimaryAndSecondary: Story = {
  args: {
    primary: { label: "Сдать", onClick: fn() },
    secondary: { label: "К списку", onClick: fn() },
  },
};

export const PrimaryDisabled: Story = {
  args: {
    primary: { label: "Сдать", onClick: fn(), disabled: true },
    secondary: { label: "К списку", onClick: fn() },
  },
};

export const PrimaryLoading: Story = {
  args: {
    primary: { label: "Сохранение…", onClick: fn(), loading: true },
  },
};

export const SecondaryDisabled: Story = {
  args: {
    primary: { label: "Выдать", onClick: fn() },
    secondary: { label: "Отмена", onClick: fn(), disabled: true },
  },
};
