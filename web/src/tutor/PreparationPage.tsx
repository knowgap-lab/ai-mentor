import { Button, Stack, Text } from "@mantine/core";
import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { Screen } from "../ui";
import { errorSentence, isRecord } from "./api-error";

// Ключ и решение уже в первом ответе, но в DOM их нет до «показать» (WB-R22).

type Item = {
  variantId: string;
  statement: string;
  answerKey: string;
  solution: string;
  options: string[];
};

type LoadResult =
  | { phase: "offline" }
  | { phase: "error"; sentence: string | null }
  | { phase: "ready"; items: Item[] };

function readKey(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "";
}

function readItem(value: unknown): Item | null {
  if (!isRecord(value)) {
    return null;
  }
  if (
    typeof value.variant_id !== "string" ||
    typeof value.statement !== "string"
  ) {
    return null;
  }
  const options: string[] = [];
  if (Array.isArray(value.options)) {
    for (const option of value.options) {
      if (isRecord(option) && typeof option.text === "string") {
        options.push(option.text);
      }
    }
  }
  return {
    variantId: value.variant_id,
    statement: value.statement,
    answerKey: readKey(value.key),
    solution:
      typeof value.solution_block === "string" ? value.solution_block : "",
    options,
  };
}

function readItems(value: unknown): Item[] | null {
  if (!isRecord(value) || !Array.isArray(value.items)) {
    return null;
  }
  const items: Item[] = [];
  for (const entry of value.items) {
    const item = readItem(entry);
    if (item) {
      items.push(item);
    }
  }
  return items;
}

async function loadPreparation(lessonId: string): Promise<LoadResult> {
  try {
    const response = await fetch(
      `/lessons/${encodeURIComponent(lessonId)}/preparation`,
      { credentials: "include" },
    );
    if (!response.ok) {
      return { phase: "error", sentence: await errorSentence(response) };
    }
    const items = readItems(await response.json());
    if (!items) {
      return { phase: "error", sentence: null };
    }
    return { phase: "ready", items };
  } catch {
    return { phase: "offline" };
  }
}

export function PreparationPage() {
  const { lessonId } = useParams();
  const [reloadKey, setReloadKey] = useState(0);
  const [phase, setPhase] = useState<"loading" | "offline" | "error" | "ready">(
    "loading",
  );
  const [errorText, setErrorText] = useState<string | null>(null);
  const [items, setItems] = useState<Item[] | null>(null);
  const [revealed, setRevealed] = useState(false);

  useEffect(() => {
    if (!lessonId) {
      return;
    }
    let cancelled = false;
    setPhase("loading");
    setRevealed(false);
    setItems(null);
    void loadPreparation(lessonId).then((result) => {
      if (cancelled) {
        return;
      }
      if (result.phase === "ready") {
        setItems(result.items);
        setPhase("ready");
        return;
      }
      setItems(null);
      if (result.phase === "offline") {
        setPhase("offline");
        return;
      }
      setErrorText(result.sentence);
      setPhase("error");
    });
    return () => {
      cancelled = true;
    };
  }, [lessonId, reloadKey]);

  return (
    <Screen title="Подготовка">
      <Stack gap="md">
        {phase === "loading" ? <Text>Загрузка</Text> : null}
        {phase === "offline" ? (
          <Stack gap="sm">
            <Text>Нет связи</Text>
            <Button
              type="button"
              variant="default"
              onClick={() => setReloadKey((value) => value + 1)}
            >
              Повторить
            </Button>
          </Stack>
        ) : null}
        {phase === "error" && errorText ? <Text>{errorText}</Text> : null}
        {phase === "ready" && items ? (
          <>
            {items.map((item, index) => (
              <Stack key={`${item.variantId}-${index}`} gap={0}>
                <Text>{item.statement}</Text>
                <Text>{item.variantId}</Text>
                {item.options.map((option, optionIndex) => (
                  <Text key={`${option}-${optionIndex}`}>{option}</Text>
                ))}
                {revealed && item.answerKey !== "" ? (
                  <Text>{item.answerKey}</Text>
                ) : null}
                {revealed && item.solution !== "" ? (
                  <Text>{item.solution}</Text>
                ) : null}
              </Stack>
            ))}
            {items.length > 0 ? (
              <Button
                type="button"
                variant="default"
                onClick={() => setRevealed(true)}
              >
                показать
              </Button>
            ) : null}
          </>
        ) : null}
      </Stack>
    </Screen>
  );
}
