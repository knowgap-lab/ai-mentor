import { expect, type Page } from "@playwright/test";

export const VIEWPORT_MOBILE = { width: 360, height: 800 };

let lastLoginMs = 0;

export async function loginDev(
  page: Page,
  origin: string,
  telegramId: string,
  role: "student" | "tutor",
): Promise<void> {
  const gap = Date.now() - lastLoginMs;
  if (gap < 1200) {
    await page.waitForTimeout(1200 - gap);
  }
  lastLoginMs = Date.now();
  await page.goto(`${origin}/`);
  await page.evaluate(
    async ({
      telegramId: tg,
      role: userRole,
    }: {
      telegramId: string;
      role: string;
    }) => {
      const response = await fetch("/auth/dev", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ telegram_id: tg, role: userRole }),
      });
      const csrf = response.headers.get("X-CSRF-Token");
      if (csrf) {
        sessionStorage.setItem("ai-mentor.csrf", csrf);
      }
      if (!response.ok) {
        throw new Error(`${response.status} ${await response.text()}`);
      }
    },
    { telegramId, role },
  );
}

export async function loginStudent(
  page: Page,
  origin: string,
): Promise<void> {
  await loginDev(page, origin, "tg-demo-student", "student");
}

export async function loginTutor(page: Page, origin: string): Promise<void> {
  await loginDev(page, origin, "tg-demo-tutor", "tutor");
}

export function watchConsole(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      errors.push(message.text());
    }
  });
  page.on("pageerror", (error) => {
    errors.push(error.message);
  });
  return errors;
}

export async function expectNoHorizontalScroll(page: Page): Promise<void> {
  const overflow = await page.evaluate(() => {
    const doc = document.documentElement;
    return doc.scrollWidth > doc.clientWidth + 1;
  });
  expect(overflow, "horizontal scroll on viewport").toBe(false);
}

export async function expectTouchTargetAtLeast(
  page: Page,
  role: "button" | "radio",
  name: RegExp | string,
  minPx = 44,
): Promise<void> {
  const locator = page.getByRole(role, { name });
  await expect(locator.first()).toBeVisible();
  const box = await locator.first().boundingBox();
  expect(box).not.toBeNull();
  if (box) {
    expect(Math.min(box.width, box.height), `${role} ${String(name)}`).toBeGreaterThanOrEqual(
      minPx - 1,
    );
  }
}