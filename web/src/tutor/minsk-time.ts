// WB-R2: интерфейс показывает время в Europe/Minsk, сервер хранит UTC.
export function minskTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return new Intl.DateTimeFormat("ru", {
    timeZone: "Europe/Minsk",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(date);
}

// PV-R12: возраст на момент создания ссылки, граница — 18 лет.
export function isUnderEighteen(birthDate: string, now = new Date()): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(birthDate);
  if (!match) {
    return false;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const today = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Minsk",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(now);
  const [todayYear, todayMonth, todayDay] = today.split("-").map(Number);
  let age = (todayYear ?? 0) - year;
  if (
    (todayMonth ?? 0) < month ||
    ((todayMonth ?? 0) === month && (todayDay ?? 0) < day)
  ) {
    age -= 1;
  }
  return age < 18;
}
