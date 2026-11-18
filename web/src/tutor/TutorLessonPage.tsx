import { Button, Radio, Stack, Text, TextInput } from "@mantine/core";
import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { csrfHeaders } from "../task/csrf";
import { ChoiceRow, Screen } from "../ui";
import { errorSentence, isRecord } from "./api-error";
import { minskLocalToUtc, minskTime } from "./minsk-time";

// Слова для planned, frozen и held в web.md нет: показываем только «пропущен».
// Подписи поля переноса в web.md нет: одно поле, местное время Минска.

const THEORY_MARK = "последний объяснённый блок теории";
const PRACTICE_MARK = "последняя решённая задача практики";
const NONE = "none";

type LessonCore = {
  id: string;
  title: string;
  kind: string;
  placeNo: number;
  topics: string[];
  startsAt?: string;
  plannedWeek?: string;
  moved: boolean;
  added: boolean;
  skipped: boolean;
};

type PracticeItem = {
  variantId: string;
  stageId: string;
  position: number;
};

type StageView = {
  id: string;
  title: string;
  kind: string;
  minutes: number[];
  blocks: string[];
  practice: PracticeItem[];
};

type Source = "snapshot" | "l_file" | "";

type LessonView = {
  lesson: LessonCore;
  carriedBlocks: string[];
  fromLessonId?: string;
  stages: StageView[];
  source: Source;
};

type LoadResult =
  | { phase: "offline" }
  | { phase: "error"; sentence: string | null }
  | { phase: "ready"; detail: LessonView };

type PracticeMark = { stage_id: string; position: number };

function readLesson(value: unknown): LessonCore | null {
  if (!isRecord(value)) {
    return null;
  }
  if (typeof value.id !== "string" || typeof value.title !== "string") {
    return null;
  }
  if (typeof value.place_no !== "number") {
    return null;
  }
  const topics = Array.isArray(value.topics)
    ? value.topics.filter((topic): topic is string => typeof topic === "string")
    : [];
  return {
    id: value.id,
    title: value.title,
    kind: typeof value.kind === "string" ? value.kind : "",
    placeNo: value.place_no,
    topics,
    startsAt: typeof value.starts_at === "string" ? value.starts_at : undefined,
    plannedWeek:
      typeof value.planned_week === "string" ? value.planned_week : undefined,
    moved: value.moved === true,
    added: value.added === true,
    skipped: value.state === "skipped",
  };
}

function readPractice(
  stageId: string,
  value: unknown,
  index: number,
): PracticeItem | null {
  if (!isRecord(value) || typeof value.variant_id !== "string") {
    return null;
  }
  const position =
    typeof value.position === "number" && value.position >= 1
      ? value.position
      : index + 1;
  return { variantId: value.variant_id, stageId, position };
}

function readStage(value: unknown): StageView | null {
  if (!isRecord(value)) {
    return null;
  }
  if (typeof value.id !== "string" || typeof value.title !== "string") {
    return null;
  }
  const minutes = Array.isArray(value.minutes)
    ? value.minutes.filter((item): item is number => typeof item === "number")
    : [];
  const blocks = Array.isArray(value.blocks)
    ? value.blocks.filter((item): item is string => typeof item === "string")
    : [];
  const stageId = value.id;
  const practice: PracticeItem[] = [];
  if (Array.isArray(value.practice)) {
    value.practice.forEach((item, index) => {
      const row = readPractice(stageId, item, index);
      if (row) {
        practice.push(row);
      }
    });
  }
  return {
    id: stageId,
    title: value.title,
    kind: typeof value.kind === "string" ? value.kind : "",
    minutes,
    blocks,
    practice,
  };
}

function readDetail(value: unknown): LessonView | null {
  if (!isRecord(value)) {
    return null;
  }
  const lesson = readLesson(value.lesson);
  if (!lesson) {
    return null;
  }
  const carriedBlocks: string[] = [];
  let fromLessonId: string | undefined;
  if (isRecord(value.carried_in)) {
    if (Array.isArray(value.carried_in.blocks)) {
      for (const block of value.carried_in.blocks) {
        if (typeof block === "string") {
          carriedBlocks.push(block);
        }
      }
    }
    if (typeof value.carried_in.from_lesson_id === "string") {
      fromLessonId = value.carried_in.from_lesson_id;
    }
  }
  const stages: StageView[] = [];
  if (Array.isArray(value.stages)) {
    for (const stage of value.stages) {
      const row = readStage(stage);
      if (row) {
        stages.push(row);
      }
    }
  }
  const source: Source =
    value.source === "snapshot" || value.source === "l_file"
      ? value.source
      : "";
  return { lesson, carriedBlocks, fromLessonId, stages, source };
}

function lessonFromPlan(value: unknown, lessonId: string): LessonCore | null {
  if (!isRecord(value) || !Array.isArray(value.lessons)) {
    return null;
  }
  for (const item of value.lessons) {
    const lesson = readLesson(item);
    if (lesson?.id === lessonId) {
      return lesson;
    }
  }
  return null;
}

function theoryIds(detail: LessonView): string[] {
  const seen = new Set<string>();
  const ids: string[] = [];
  const push = (id: string) => {
    if (seen.has(id)) {
      return;
    }
    seen.add(id);
    ids.push(id);
  };
  for (const block of detail.carriedBlocks) {
    push(block);
  }
  for (const stage of detail.stages) {
    for (const block of stage.blocks) {
      push(block);
    }
  }
  return ids;
}

function practiceItems(detail: LessonView): PracticeItem[] {
  const items: PracticeItem[] = [];
  for (const stage of detail.stages) {
    items.push(...stage.practice);
  }
  return items;
}

function sourceLabel(source: Source): string {
  if (source === "snapshot") {
    return "снимок";
  }
  if (source === "l_file") {
    return "файл урока";
  }
  return "";
}

async function loadLesson(lessonId: string): Promise<LoadResult> {
  try {
    const response = await fetch(`/lessons/${encodeURIComponent(lessonId)}`, {
      credentials: "include",
    });
    if (!response.ok) {
      return { phase: "error", sentence: await errorSentence(response) };
    }
    const detail = readDetail(await response.json());
    if (!detail) {
      return { phase: "error", sentence: null };
    }
    return { phase: "ready", detail };
  } catch {
    return { phase: "offline" };
  }
}

async function postJson(path: string, body: unknown): Promise<Response> {
  return fetch(path, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...csrfHeaders(),
    },
    body: JSON.stringify(body),
  });
}

async function postEmpty(path: string): Promise<Response> {
  return fetch(path, {
    method: "POST",
    credentials: "include",
    headers: csrfHeaders(),
  });
}

function lessonUrl(lessonId: string, action?: string): string {
  const base = `/lessons/${encodeURIComponent(lessonId)}`;
  return action ? `${base}/${action}` : base;
}

export function TutorLessonPage() {
  const { lessonId } = useParams();
  const [reloadKey, setReloadKey] = useState(0);
  const [phase, setPhase] = useState<"loading" | "offline" | "error" | "ready">(
    "loading",
  );
  const [errorText, setErrorText] = useState<string | null>(null);
  const [detail, setDetail] = useState<LessonView | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [when, setWhen] = useState("");
  const [theory, setTheory] = useState("");
  const [practice, setPractice] = useState("");

  useEffect(() => {
    if (!lessonId) {
      return;
    }
    let cancelled = false;
    setPhase("loading");
    setActionError(null);
    setTheory("");
    setPractice("");
    void loadLesson(lessonId).then((result) => {
      if (cancelled) {
        return;
      }
      if (result.phase === "ready") {
        setDetail(result.detail);
        setPhase("ready");
        return;
      }
      setDetail(null);
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

  function replaceLesson(lesson: LessonCore | null) {
    if (!lesson) {
      return;
    }
    setDetail((current) => (current ? { ...current, lesson } : current));
  }

  async function onMove() {
    if (!lessonId) {
      return;
    }
    const startsAt = minskLocalToUtc(when);
    if (!startsAt) {
      return;
    }
    setActionError(null);
    try {
      const response = await postJson(lessonUrl(lessonId, "move"), {
        starts_at: startsAt,
      });
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      replaceLesson(readLesson(await response.json()));
    } catch {
      setActionError("Нет связи");
    }
  }

  async function onPlan(action: "cancel" | "skip") {
    if (!lessonId) {
      return;
    }
    setActionError(null);
    try {
      const response = await postEmpty(lessonUrl(lessonId, action));
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      replaceLesson(lessonFromPlan(await response.json(), lessonId));
    } catch {
      setActionError("Нет связи");
    }
  }

  async function postMarks(body: unknown) {
    if (!lessonId) {
      return;
    }
    setActionError(null);
    try {
      const response = await postJson(lessonUrl(lessonId, "marks"), body);
      if (!response.ok) {
        setActionError(await errorSentence(response));
        return;
      }
      const next = readDetail(await response.json());
      if (next) {
        setDetail(next);
      }
    } catch {
      setActionError("Нет связи");
    }
  }

  function onMarks() {
    if (!detail || theory === "" || practice === "") {
      return;
    }
    const items = practiceItems(detail);
    let lastPractice: PracticeMark | null = null;
    if (practice !== NONE) {
      const item = items[Number(practice)];
      if (!item) {
        return;
      }
      lastPractice = { stage_id: item.stageId, position: item.position };
    }
    void postMarks({
      last_theory_block: theory === NONE ? null : theory,
      last_practice: lastPractice,
    });
  }

  const lesson = detail?.lesson;
  const blocks = detail ? theoryIds(detail) : [];
  const tasks = detail ? practiceItems(detail) : [];

  return (
    <Screen title="Урок">
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
        {phase === "ready" && lesson && detail ? (
          <>
            <Text>{lesson.title}</Text>
            {lesson.kind ? <Text>{lesson.kind}</Text> : null}
            {lesson.topics.map((topic) => (
              <Text key={topic}>{topic}</Text>
            ))}
            <Text>{lesson.placeNo}</Text>
            {lesson.startsAt ? <Text>{minskTime(lesson.startsAt)}</Text> : null}
            {!lesson.startsAt && lesson.plannedWeek ? (
              <Text>{`неделя с понедельника ${lesson.plannedWeek}`}</Text>
            ) : null}
            {lesson.moved ? <Text>перенесено</Text> : null}
            {lesson.added ? <Text>добавлено</Text> : null}
            {lesson.skipped ? <Text>пропущен</Text> : null}
            {sourceLabel(detail.source) ? (
              <Text>{sourceLabel(detail.source)}</Text>
            ) : null}
            {detail.carriedBlocks.map((block) => (
              <Text key={`carried-${block}`}>{block}</Text>
            ))}
            {detail.fromLessonId ? <Text>{detail.fromLessonId}</Text> : null}
            {detail.stages.map((stage) => (
              <Stack key={stage.id} gap={0}>
                <Text>{stage.title}</Text>
                {stage.kind ? <Text>{stage.kind}</Text> : null}
                {stage.minutes.length > 0 ? (
                  <Text>{stage.minutes.join("-")}</Text>
                ) : null}
                {stage.blocks.map((block) => (
                  <Text key={block}>{block}</Text>
                ))}
                {stage.practice.map((item) => (
                  <Text key={`${item.stageId}-${item.position}`}>
                    {item.variantId}
                  </Text>
                ))}
              </Stack>
            ))}
            {detail.source === "snapshot" ? (
              <>
                <Radio.Group
                  label={THEORY_MARK}
                  name="theory-mark"
                  value={theory}
                  onChange={setTheory}
                >
                  <Stack gap="sm">
                    {blocks.map((block) => (
                      <ChoiceRow key={block}>
                        <Radio value={block} label={block} />
                      </ChoiceRow>
                    ))}
                    <ChoiceRow>
                      <Radio value={NONE} label="нет" />
                    </ChoiceRow>
                  </Stack>
                </Radio.Group>
                <Radio.Group
                  label={PRACTICE_MARK}
                  name="practice-mark"
                  value={practice}
                  onChange={setPractice}
                >
                  <Stack gap="sm">
                    {tasks.map((item, index) => (
                      <ChoiceRow key={`${item.stageId}-${item.position}`}>
                        <Radio value={String(index)} label={item.variantId} />
                      </ChoiceRow>
                    ))}
                    <ChoiceRow>
                      <Radio value={NONE} label="нет" />
                    </ChoiceRow>
                  </Stack>
                </Radio.Group>
                <Button type="button" variant="default" onClick={onMarks}>
                  две отметки
                </Button>
              </>
            ) : null}
            {detail.source === "l_file" ? (
              <>
                <Button
                  type="button"
                  variant="default"
                  onClick={() => void postMarks({ coverage: "all" })}
                >
                  пройдено всё
                </Button>
                <Button
                  type="button"
                  variant="default"
                  onClick={() => void postMarks({ coverage: "none" })}
                >
                  не пройдено ничего
                </Button>
              </>
            ) : null}
            <TextInput
              type="text"
              aria-label="время"
              value={when}
              onChange={(event) => setWhen(event.currentTarget.value)}
            />
            <Button
              type="button"
              variant="default"
              onClick={() => void onMove()}
            >
              перенос
            </Button>
            <Button
              type="button"
              variant="default"
              onClick={() => void onPlan("cancel")}
            >
              отмена
            </Button>
            <Button
              type="button"
              variant="default"
              onClick={() => void onPlan("skip")}
            >
              пропуск
            </Button>
            {actionError ? <Text>{actionError}</Text> : null}
          </>
        ) : null}
      </Stack>
    </Screen>
  );
}
