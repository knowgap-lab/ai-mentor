const STORAGE_KEY = "ai-mentor.csrf";
const HEADER = "X-CSRF-Token";

function readToken(): string | null {
  try {
    return sessionStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function writeToken(token: string): void {
  try {
    sessionStorage.setItem(STORAGE_KEY, token);
  } catch {
    // Сеанс в памяти вкладки недоступен — запрос уйдёт без заголовка.
  }
}

// Токен берётся только из заголовка ответа входа, не из полей тела Me.
export function rememberCsrfFromResponse(response: Response): void {
  const token = response.headers.get(HEADER);
  if (!token) {
    return;
  }
  writeToken(token);
}

export function csrfHeaders(): Record<string, string> {
  const token = readToken();
  if (!token) {
    return {};
  }
  return { [HEADER]: token };
}
