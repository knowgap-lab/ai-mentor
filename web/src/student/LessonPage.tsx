import { Button, Stack, Text } from "@mantine/core";
import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { ActionBar, Screen, TaskSheet } from "../ui";
import { Statement } from "../task/Statement";
import { rememberCsrfFromResponse } from "../task/csrf";

type Block = {
  id: string;
  markdown: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function readBlock(value: unknown): Block | null {
  if (!isRecord(value)) {
    return null;
  }
  if (typeof value.id !== "string" || typeof value.markdown !== "string") {
    return null;
  }
  return { id: value.id, markdown: value.markdown };
}

function readBlocks(value: unknown): Block[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item) => readBlock(item))
    .filter((item): item is Block => item !== null);
}

function readConspect(value: unknown): Block[] | null {
  if (!isRecord(value)) {
    return null;
  }
  return [...readBlocks(value.carried_over), ...readBlocks(value.blocks)];
}

function hiddenBlock(markdown: string): boolean {
  return /<\s*script\b/i.test(markdown);
}

export function LessonPage() {
  const { lessonId } = useParams();
  const [blocks, setBlocks] = useState<Block[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const [pdfError, setPdfError] = useState<string | null>(null);

  useEffect(() => {
    if (!lessonId) {
      return;
    }
    let cancelled = false;
    async function load() {
      setLoading(true);
      setFailed(false);
      try {
        const response = await fetch(
          `/lessons/${encodeURIComponent(lessonId ?? "")}/conspect`,
          { credentials: "include" },
        );
        rememberCsrfFromResponse(response);
        if (!response.ok) {
          throw new Error("load failed");
        }
        const payload: unknown = await response.json();
        const next = readConspect(payload);
        if (!next) {
          throw new Error("load failed");
        }
        if (!cancelled) {
          setBlocks(next);
          setLoading(false);
        }
      } catch {
        if (!cancelled) {
          setBlocks(null);
          setFailed(true);
          setLoading(false);
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [lessonId, reloadKey]);

  async function onPdf() {
    if (!lessonId) {
      return;
    }
    setPdfError(null);
    try {
      const response = await fetch(
        `/lessons/${encodeURIComponent(lessonId)}/conspect.pdf`,
        { credentials: "include" },
      );
      rememberCsrfFromResponse(response);
      if (!response.ok) {
        // Кода pdf_unavailable нет в таблице ошибок: тело ответа не показываем.
        setPdfError("PDF сейчас недоступен");
        return;
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = "conspect.pdf";
      link.click();
      URL.revokeObjectURL(url);
    } catch {
      setPdfError("Нет связи");
    }
  }

  if (!lessonId) {
    return (
      <Screen title="Конспект" backTo="/">
        <Text>Урок не найден</Text>
      </Screen>
    );
  }

  const visible = (blocks ?? []).filter(
    (block) => !hiddenBlock(block.markdown),
  );
  const canPdf = blocks !== null;

  return (
    <Screen title="Конспект" backTo="/" withActionBar={canPdf}>
      <Stack gap="md">
        {loading ? <Text>Загрузка</Text> : null}
        {failed ? (
          <Stack gap="sm">
            <Text>Нет связи</Text>
            <Button
              variant="default"
              onClick={() => setReloadKey((value) => value + 1)}
            >
              Повторить
            </Button>
          </Stack>
        ) : null}
        {blocks && visible.length === 0 ? <Text>Пусто</Text> : null}
        {visible.map((block) => (
          <TaskSheet key={block.id} label="Конспект">
            <Statement source={block.markdown} />
          </TaskSheet>
        ))}
        {pdfError ? <Text>{pdfError}</Text> : null}
      </Stack>
      {canPdf ? (
        <ActionBar
          primary={{ label: "Скачать PDF", onClick: () => void onPdf() }}
        />
      ) : null}
    </Screen>
  );
}
