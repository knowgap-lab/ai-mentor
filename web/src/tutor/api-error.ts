// Тексты — таблица «Ошибки» в web.md. Код и техническое сообщение не показываем (WB-R6).
const ERROR_TEXT: Record<string, string> = {
  item_closed: "Эта задача уже закрыта",
  assignment_closed: "ДЗ уже сдано",
  rate_limited: "Слишком часто, подождите немного",
  consent_missing: "Нужно согласие родителя — обратитесь к репетитору",
  invite_gone: "Приглашение недействительно — попросите новое",
  login_stale: "Войдите ещё раз",
  device_pending: "Подтвердите вход в Telegram",
  has_answers: "Нельзя отозвать: ученик уже отвечал",
  unavailable: "Нет связи, попробуйте ещё раз",
  account_locked: "Вход закрыт — обратитесь к репетитору",
  login_invalid: "Не удалось войти — попробуйте ещё раз",
  csrf: "Обновите страницу и повторите",
  forbidden: "Нет доступа или объекта не существует",
  telegram_already_bound: "Этот Telegram уже привязан к другому пользователю",
  consent_link_gone:
    "Ссылка согласия недействительна — попросите у репетитора новую",
  dispute_text_invalid:
    "Уберите из текста телефоны, почту и ссылки или сократите до 300 символов",
  deadline_shorter: "Срок можно только продлить",
  dispute_exists: "Спор по этой задаче уже открыт",
  not_submitted: "Спор можно открыть после сдачи ДЗ",
  idempotency_conflict:
    "Этот ответ уже отправлялся с другим текстом — обновите страницу",
};

function isRecord(value: unknown): value is Record<string, unknown> {
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
