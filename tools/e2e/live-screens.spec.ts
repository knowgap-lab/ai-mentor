import AxeBuilder from "@axe-core/playwright";
import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

const ORIGIN = process.env.LIVE_ORIGIN ?? "http://127.0.0.1:8080";
const CSP =
  "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'";

const ISSUED = "asg-demo";
const ITEM_OK = "item-a";
const LESSON = "lesson-demo-held";
const LESSON_NEXT = "lesson-demo-next";
const STUDENT = "student-demo";

type ScreenCase = {
  key: string;
  path: string;
  probe?: (page: Page) => Promise<void>;
};

const STUDENT_SCREENS: ScreenCase[] = [
  { key: "Ученик/Главная", path: "/" },
  { key: "Ученик/Урок", path: `/lesson/${LESSON}` },
  { key: "Ученик/Выдача (ДЗ)", path: `/assignment/${ISSUED}` },
  { key: "Ученик/Задача", path: `/task/${ISSUED}/${ITEM_OK}` },
  { key: "Ученик/Итог", path: `/assignment/${ISSUED}/summary` },
  { key: "Ученик/Профиль", path: "/profile" },
];

const TUTOR_SCREENS: ScreenCase[] = [
  { key: "Репетитор/Ученики", path: "/tutor" },
  { key: "Репетитор/Ученик", path: `/tutor/students/${STUDENT}` },
  { key: "Репетитор/Урок", path: `/tutor/lessons/${LESSON}` },
  { key: "Репетитор/Подготовка", path: `/tutor/lessons/${LESSON}/preparation` },
  { key: "Репетитор/Просмотр ДЗ", path: `/tutor/lessons/${LESSON_NEXT}/homework` },
  { key: "Репетитор/Итоги ДЗ", path: `/tutor/students/${STUDENT}/assignments/asg-demo-submitted` },
  { key: "Репетитор/Новый ученик", path: "/tutor/students/new" },
  { key: "Репетитор/Сеансы", path: "/tutor/sessions" },
];

function watch(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (message) => {
    const text = message.text();
    if (message.type() === "error" || /content security policy/i.test(text)) {
      if (/Failed to load resource/i.test(text)) {
        return;
      }
      errors.push(`${message.type()}: ${text}`);
    }
  });
  page.on("pageerror", (error) => errors.push(error.message));
  return errors;
}

async function sessionFromRequest(
  request: APIRequestContext,
  telegram: string,
  role: string,
) {
  const response = await request.post(`${ORIGIN}/auth/dev`, {
    headers: { Origin: ORIGIN, "Content-Type": "application/json" },
    data: { telegram_id: telegram, role },
  });
  expect(response.ok()).toBeTruthy();
  return (await request.storageState()).cookies;
}

async function expectAccessible(page: Page, errors: string[]) {
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === "serious" || v.impact === "critical",
  );
  expect(serious).toEqual([]);
  expect(errors).toEqual([]);
}

async function layoutChecks(page: Page) {
  const metrics = await page.evaluate(() => {
    const doc = document.documentElement;
    const buttons = [...document.querySelectorAll("button")];
    const small = buttons.filter((b) => {
      const rect = b.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && rect.height < 44;
    }).length;
    return {
      horizontal: doc.scrollWidth > doc.clientWidth + 1,
      smallButtons: small,
    };
  });
  expect(metrics.horizontal).toBeFalsy();
  expect(metrics.smallButtons).toBe(0);
}

async function visitScreens(
  page: Page,
  screens: ScreenCase[],
  options?: { skipGoto?: string },
) {
  for (const screen of screens) {
    const errors = watch(page);
    const skip = options?.skipGoto === screen.path;
    const response = skip ? null : await page.goto(ORIGIN + screen.path);
    if (response) {
      expect(response.headers()["content-security-policy"]).toBe(CSP);
    }
    await expect(page.getByRole("main")).toBeVisible();
    await layoutChecks(page);
    await expectAccessible(page, errors);
  }
}

async function tutorLogin(page: Page) {
  await page.goto(`${ORIGIN}/tutor`);
  await page.getByRole("textbox").fill("tg-demo-tutor");
  await page.getByRole("button", { name: "Войти" }).click();
  await expect(page.getByRole("button", { name: /student-demo/ })).toBeVisible();
}

test.describe.configure({ mode: "serial" });

test("student mandatory screens on live stack", async ({ browser, request }) => {
  const cookies = await sessionFromRequest(request, "tg-demo-student", "student");
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    reducedMotion: "reduce",
  });
  await context.addCookies(cookies);
  const page = await context.newPage();
  await visitScreens(page, STUDENT_SCREENS);
  const errors = watch(page);
  const ready = page.waitForResponse((r) => {
    const u = new URL(r.url());
    return u.pathname === `/assignments/${ISSUED}` && r.request().method() === "GET";
  });
  await page.goto(`${ORIGIN}/task/${ISSUED}/${ITEM_OK}`);
  expect((await ready).text()).not.toContain('"key"');
  await page.getByRole("radio", { name: "3. 3", exact: true }).check();
  await page.getByRole("button", { name: "Ответить" }).click();
  await expect(page.getByText("верно", { exact: true })).toBeVisible();
  await expectAccessible(page, errors);
  await context.close();
});

test("tutor mandatory screens on live stack", async ({ browser }) => {
  await new Promise((r) => setTimeout(r, 10_000));
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    reducedMotion: "reduce",
  });
  const page = await context.newPage();
  await tutorLogin(page);
  await visitScreens(page, TUTOR_SCREENS, { skipGoto: "/tutor" });
  await context.close();
});