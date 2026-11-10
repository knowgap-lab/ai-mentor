import { Button, Group, Stack, Text, TextInput, Title } from "@mantine/core";
import "katex/dist/katex.min.css";
import ReactMarkdown from "react-markdown";
import rehypeKatex from "rehype-katex";
import remarkMath from "remark-math";
import {
  ActionBar,
  List,
  ListRow,
  Screen,
  STATUSES,
  StatusBadge,
  TaskSheet,
  type Status,
} from "../ui";

// Образец дизайн-системы (docs/platform/design.md). Подключается только в `vite dev`.
const statement =
  "Найдите наименьшее целое решение неравенства $\\dfrac{2x-3}{x+4} \\ge 1$.";

export function DesignPage() {
  return (
    <Screen title="Образец" backTo="/" withActionBar>
      <Stack gap="lg">
        <section aria-label="Состояния">
          <Title order={2} mb="sm">
            Состояния
          </Title>
          <Group gap="sm">
            {(Object.keys(STATUSES) as Status[]).map((state) => (
              <StatusBadge key={state} state={state} />
            ))}
          </Group>
        </section>

        <section aria-label="Список">
          <Title order={2} mb="sm">
            Выдачи
          </Title>
          <List label="Выдачи">
            <ListRow
              to="/design"
              title="ДЗ к уроку 12"
              subtitle="Математика · срок 14 ноября · 3 из 8"
              aside={<StatusBadge state="not_answered" />}
            />
            <ListRow
              to="/design"
              title="ДЗ к уроку 11"
              subtitle="Математика · сдано 9 ноября · 6 из 8"
              aside={<StatusBadge state="submitted" />}
            />
            <ListRow
              to="/design"
              title="Срез по теме «Неравенства»"
              subtitle="Математика · срок прошёл"
              aside={<StatusBadge state="overdue" />}
            />
          </List>
        </section>

        <section aria-label="Задача">
          <Title order={2} mb="sm">
            Задача
          </Title>
          <TaskSheet label="Задача 4 из 8 · часть В · осталась 1 попытка">
            <ReactMarkdown
              remarkPlugins={[remarkMath]}
              rehypePlugins={[rehypeKatex]}
            >
              {statement}
            </ReactMarkdown>
            <TextInput
              label="Ответ"
              inputMode="numeric"
              placeholder="целое число"
              mt="md"
            />
            <Group gap="sm" mt="sm">
              <StatusBadge state="wrong" />
              <Text size="sm" c="dimmed">
                неверно, осталась 1 попытка
              </Text>
            </Group>
          </TaskSheet>
        </section>

        <section aria-label="Текст и кнопки">
          <Title order={2} mb="sm">
            Текст и кнопки
          </Title>
          <Text>
            Основной текст 16 px, межстрочный 1.5. Баллы табличными цифрами:{" "}
            <span className="am-num">14 / 20</span>.
          </Text>
          <Text size="sm" c="dimmed" mt="xs">
            Второстепенный текст 14 px.
          </Text>
          <Group gap="sm" mt="md">
            <Button>Ответить</Button>
            <Button variant="default">Не понял</Button>
            <Button variant="subtle" color="red">
              Оспорить
            </Button>
          </Group>
        </section>
      </Stack>
      <ActionBar
        primary={{ label: "Сдать", onClick: () => undefined }}
        secondary={{ label: "К списку", onClick: () => undefined }}
      />
    </Screen>
  );
}
