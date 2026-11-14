// Тексты из таблицы «Ошибки» web.md. Код и message API на экран не попадают (WB-R6).

const TEXTS: Record<string, string> = {
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
};

export const OFFLINE_TEXT = "Нет связи";

export async function messageForResponse(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { code?: unknown };
    if (typeof body.code === "string") {
      const text = TEXTS[body.code];
      if (text) {
        return text;
      }
    }
  } catch {
    return OFFLINE_TEXT;
  }
  return OFFLINE_TEXT;
}
