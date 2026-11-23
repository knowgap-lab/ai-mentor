import { chromium, expect, test, type BrowserContext } from "@playwright/test";
import {
  expectNoHorizontalScroll,
  expectTouchTargetAtLeast,
  loginStudent,
  loginTutor,
  VIEWPORT_MOBILE,
} from "./helpers";
import { startStack, type Stack } from "./stack";

let stack: Stack;
let studentContext: BrowserContext;
let tutorContext: BrowserContext;

const LESSON_HELD = "lesson-demo-held";
const ASG_OPEN = "asg-demo";
const ASG_SUBMITTED = "asg-demo-submitted";
const STUDENT = "student-demo";

test.beforeAll(async () => {
  stack = await startStack();
  const browser = await chromium.launch({ channel: "chrome" });
  studentContext = await browser.newContext({ viewport: VIEWPORT_MOBILE });
  const studentPage = await studentContext.newPage();
  await loginStudent(studentPage, stack.origin);
  await studentPage.close();

  tutorContext = await browser.newContext({ viewport: VIEWPORT_MOBILE });
  const tutorPage = await tutorContext.newPage();
  await loginTutor(tutorPage, stack.origin);
  await tutorPage.close();
});

test.afterAll(async () => {
  await studentContext?.close();
  await tutorContext?.close();
  await stack?.stop();
});

test.describe.configure({ mode: "serial", timeout: 120_000 });

test("Ученик/Урок — конспект и PDF", async () => {
  const page = await studentContext.newPage();
  await page.goto(`${stack.origin}/lesson/${LESSON_HELD}`);
  await expect(page.getByRole("button", { name: "Скачать PDF" })).toBeVisible({
    timeout: 60_000,
  });
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Ученик/Выдача (ДЗ) — список задач", async () => {
  const page = await studentContext.newPage();
  await page.goto(`${stack.origin}/assignment/${ASG_OPEN}`);
  await expect(page.getByRole("button", { name: "Сдать" })).toBeVisible({
    timeout: 60_000,
  });
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Ученик/Итог — итог сданного ДЗ", async () => {
  const page = await studentContext.newPage();
  await page.goto(`${stack.origin}/assignment/${ASG_SUBMITTED}/summary`);
  await expect(page.getByRole("button", { name: "не понял" }).first()).toBeVisible({
    timeout: 60_000,
  });
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Ученик/Профиль — имя и выход", async () => {
  const page = await studentContext.newPage();
  await page.goto(`${stack.origin}/profile`);
  await expect(page.getByRole("button", { name: "Выйти" })).toBeVisible({
    timeout: 60_000,
  });
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Репетитор/Ученик — карточка по маршруту", async () => {
  const page = await tutorContext.newPage();
  await page.goto(`${stack.origin}/tutor/students/${STUDENT}`);
  await expect(page).toHaveURL(/\/tutor\/students\/student-demo$/);
  await expect(page.getByRole("button", { name: "Приглашение" })).toBeVisible({
    timeout: 60_000,
  });
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Репетитор/Урок — состав урока", async () => {
  const page = await tutorContext.newPage();
  await page.goto(`${stack.origin}/tutor/lessons/${LESSON_HELD}`);
  await expect(page.getByText("Демо: сложение")).toBeVisible({
    timeout: 60_000,
  });
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Репетитор/Подготовка — ключи за «показать»", async () => {
  const page = await tutorContext.newPage();
  await page.goto(`${stack.origin}/tutor/lessons/${LESSON_HELD}/preparation`);
  await expect(
    page.getByRole("button", { name: "показать" }).first(),
  ).toBeVisible({ timeout: 60_000 });
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Репетитор/Просмотр ДЗ — черновик", async () => {
  const page = await tutorContext.newPage();
  await page.goto(`${stack.origin}/tutor/lessons/${LESSON_HELD}/homework`);
  await expect(
    page
      .getByRole("button", { name: "Выдать" })
      .or(page.getByRole("button", { name: "Отозвать" })),
  ).toBeVisible({ timeout: 60_000 });
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Репетитор/Итоги ДЗ — сырой ввод", async () => {
  const page = await tutorContext.newPage();
  await page.goto(
    `${stack.origin}/tutor/students/${STUDENT}/assignments/${ASG_SUBMITTED}`,
  );
  await expect(page.getByText("5", { exact: true })).toBeVisible({
    timeout: 60_000,
  });
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Репетитор/Новый ученик — форма", async () => {
  const page = await tutorContext.newPage();
  await page.goto(`${stack.origin}/tutor/students/new`);
  await expect(page.getByRole("textbox", { name: "имя" })).toBeVisible({
    timeout: 60_000,
  });
  await expect(page.getByRole("textbox", { name: "дата рождения" })).toBeVisible();
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("Репетитор/Сеансы — список сеансов", async () => {
  const page = await tutorContext.newPage();
  await page.goto(`${stack.origin}/tutor/sessions`);
  await expect(page.getByText("Сеансы")).toBeVisible({ timeout: 60_000 });
  await expect(page.getByRole("button", { name: "отозвать все" })).toBeVisible();
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("reduced_motion — transition при reduce", async () => {
  const page = await studentContext.newPage();
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(`${stack.origin}/assignment/${ASG_OPEN}`);
  const duration = await page.evaluate(() => {
    const probe = document.createElement("div");
    probe.style.transition = "opacity 1s";
    document.body.appendChild(probe);
    const value = getComputedStyle(probe).transitionDuration;
    probe.remove();
    return value;
  });
  expect(parseFloat(duration)).toBeLessThan(0.02);
  await page.close();
});

test("dialogs — подтверждение «Сдать»", async () => {
  const page = await studentContext.newPage();
  await page.goto(`${stack.origin}/assignment/${ASG_OPEN}`);
  const dialog = page.waitForEvent("dialog");
  await page.getByRole("button", { name: "Сдать" }).click();
  const opened = await dialog;
  expect(opened.message()).toMatch(/осталось/);
  await opened.dismiss();
  await page.close();
});

test("toasts — WB-R31 подтверждение «сдано»", async () => {
  const page = await studentContext.newPage();
  await page.goto(`${stack.origin}/assignment/${ASG_OPEN}`);
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Сдать" }).click();
  await expect(page.getByText("сдано", { exact: true })).toBeVisible({
    timeout: 60_000,
  });
  await page.close();
});

test("charts — числовые показатели на «Ученики»", async () => {
  const page = await tutorContext.newPage();
  await page.goto(`${stack.origin}/tutor`);
  await expect(page.getByText(/ДЗ ждёт просмотра \d+/)).toBeVisible({
    timeout: 60_000,
  });
  await expect(page.getByText(/сдано \d+/)).toBeVisible();
  await page.close();
});

test("no_horizontal_scroll — задача на 360 px", async () => {
  const page = await studentContext.newPage();
  await page.goto(`${stack.origin}/task/${ASG_OPEN}/item-a`);
  await expectNoHorizontalScroll(page);
  await page.close();
});

test("action_size — кнопка «Ответить»", async () => {
  const page = await studentContext.newPage();
  await page.goto(`${stack.origin}/task/${ASG_OPEN}/item-a`);
  await expectTouchTargetAtLeast(page, "button", "Ответить");
  await page.close();
});

test("invite — ссылка приглашения", async () => {
  const page = await tutorContext.newPage();
  await page.goto(`${stack.origin}/tutor/students/${STUDENT}`);
  await expect(page.getByText("Демо Ученик")).toBeVisible({ timeout: 60_000 });
  await page.getByRole("button", { name: "Приглашение" }).click();
  await expect(page.getByText(/^https?:\/\//)).toBeVisible({ timeout: 60_000 });
  await page.close();
});

test("sessions — активный сеанс виден", async () => {
  const page = await tutorContext.newPage();
  await page.goto(`${stack.origin}/tutor/sessions`);
  await expect(page.locator("main").getByText(/./)).not.toHaveCount(0);
  await page.close();
});