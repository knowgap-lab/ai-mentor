// Тексты из таблицы «Ошибки» web.md. Код и message API на экран не попадают (WB-R6).

export const ERROR_TEXT: Record<string, string> = {
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
  invite_gone: "Приглашение недействительно — попросите новое",
  device_pending: "Подтвердите вход в Telegram",
  has_answers: "Нельзя отозвать: ученик уже отвечал",
  account_locked: "Вход закрыт — обратитесь к репетитору",
  login_invalid: "Не удалось войти — попробуйте ещё раз",
  telegram_already_bound: "Этот Telegram уже привязан к другому пользователю",
  consent_link_gone:
    "Ссылка согласия недействительна — попросите у репетитора новую",
  deadline_shorter: "Срок можно только продлить",
};

export const OFFLINE_TEXT = "Нет связи";

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export async function errorSentence(
  response: Response,
): Promise<string | null> {
  try {
    const payload: unknown = await response.json();
    if (!isRecord(payload) || typeof payload.code !== "string") {
      return null;
    }
    return ERROR_TEXT[payload.code] ?? null;
  } catch {
    return null;
  }
}

export async function messageForResponse(response: Response): Promise<string> {
  return (await errorSentence(response)) ?? OFFLINE_TEXT;
}