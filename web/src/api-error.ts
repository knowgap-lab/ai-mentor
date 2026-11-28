import { isRecord } from "./record";

const ERROR_TEXT: Record<string, string> = {
  item_closed: "Эта задача уже закрыта",
  assignment_closed: "ДЗ уже сдано",
  idempotency_conflict:
    "Этот ответ уже отправлялся с другим текстом — обновите страницу",
  forbidden: "Нет доступа или объекта не существует",
  csrf: "Обновите страницу и повторите",
  rate_limited: "Слишком часто, подождите немного",
  unavailable: "Нет связи, попробуйте ещё раз",
  login_stale: "Войдите ещё раз",
  dispute_exists: "Спор по этой задаче уже открыт",
  dispute_text_invalid:
    "Уберите из текста телефоны, почту и ссылки или сократите до 300 символов",
  not_submitted: "Спор можно открыть после сдачи ДЗ",
  consent_missing: "Нужно согласие родителя — обратитесь к репетитору",
  has_answers: "Нельзя отозвать: ученик уже отвечал",
  account_locked: "Вход закрыт — обратитесь к репетитору",
};

export const OFFLINE_TEXT = "Нет связи";

export function errorText(code: string): string | undefined {
  return ERROR_TEXT[code];
}

export async function errorSentence(
  response: Response,
): Promise<string | null> {
  try {
    const payload: unknown = await response.json();
    if (!isRecord(payload) || typeof payload.code !== "string") {
      return null;
    }
    return errorText(payload.code) ?? null;
  } catch {
    return null;
  }
}
