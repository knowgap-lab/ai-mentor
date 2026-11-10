export type OutboxAnswer = string | number[];

export type OutboxItem = {
  assignmentId: string;
  itemId: string;
  answer: OutboxAnswer;
  idempotency_key: string;
};

const STORAGE_KEY = "ai-mentor.outbox";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isAnswer(value: unknown): value is OutboxAnswer {
  if (typeof value === "string") {
    return true;
  }
  return (
    Array.isArray(value) && value.every((item) => typeof item === "number")
  );
}

function isItem(value: unknown): value is OutboxItem {
  if (!isRecord(value)) {
    return false;
  }
  return (
    typeof value.assignmentId === "string" &&
    typeof value.itemId === "string" &&
    typeof value.idempotency_key === "string" &&
    isAnswer(value.answer)
  );
}

export function readOutbox(): OutboxItem[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return [];
    }
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.filter((item) => isItem(item));
  } catch {
    return [];
  }
}

function writeOutbox(items: OutboxItem[]): void {
  try {
    if (items.length === 0) {
      localStorage.removeItem(STORAGE_KEY);
      return;
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
  } catch {
    // Хранилище устройства недоступно — ответ остаётся только в этом вызове.
  }
}

export function upsertOutbox(item: OutboxItem): void {
  const next = readOutbox().filter(
    (entry) =>
      entry.assignmentId !== item.assignmentId || entry.itemId !== item.itemId,
  );
  next.push(item);
  writeOutbox(next);
}

export function dropOutbox(assignmentId: string, itemId: string): void {
  writeOutbox(
    readOutbox().filter(
      (entry) => entry.assignmentId !== assignmentId || entry.itemId !== itemId,
    ),
  );
}
