import { Badge } from "@mantine/core";

// DS-R3: состояние задачи и выдачи — фиксированные подпись и цвет.
export const STATUSES = {
  not_answered: { label: "не отвечено", color: "gray", variant: "light" },
  correct: { label: "верно", color: "green", variant: "light" },
  partial: { label: "частично", color: "yellow", variant: "light" },
  wrong: { label: "неверно", color: "red", variant: "light" },
  closed: { label: "закрыто", color: "gray", variant: "filled" },
  submitted: { label: "сдано", color: "ink", variant: "light" },
  awaiting_review: {
    label: "ждёт просмотра",
    color: "ink",
    variant: "outline",
  },
  overdue: { label: "просрочено", color: "red", variant: "outline" },
} as const;

export type Status = keyof typeof STATUSES;

export function StatusBadge({ state }: { state: Status }) {
  const { label, color, variant } = STATUSES[state];
  return (
    <Badge color={color} variant={variant} radius="sm" size="md" tt="none">
      {label}
    </Badge>
  );
}
