import type { Meta, StoryObj } from "@storybook/react-vite";
import { STATUSES, StatusBadge, type Status } from "./StatusBadge";

const meta = {
  title: "UI/StatusBadge",
  component: StatusBadge,
  tags: ["autodocs"],
} satisfies Meta<typeof StatusBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

const statuses = Object.keys(STATUSES) as Status[];

export const NotAnswered: Story = { args: { state: "not_answered" } };
export const Correct: Story = { args: { state: "correct" } };
export const Partial: Story = { args: { state: "partial" } };
export const Wrong: Story = { args: { state: "wrong" } };
export const Closed: Story = { args: { state: "closed" } };
export const Submitted: Story = { args: { state: "submitted" } };
export const AwaitingReview: Story = { args: { state: "awaiting_review" } };
export const Overdue: Story = { args: { state: "overdue" } };

export const AllStates: Story = {
  args: { state: "not_answered" },
  render: () => (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
      {statuses.map((state) => (
        <StatusBadge key={state} state={state} />
      ))}
    </div>
  ),
};