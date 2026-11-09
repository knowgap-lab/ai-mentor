import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { startStack, type Stack } from "./stack";

const CSP =
  "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'";

let stack: Stack;

test.beforeAll(async () => {
  stack = await startStack();
});

test.afterAll(async () => {
  await stack?.stop();
});

function watch(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (message) => {
    const text = message.text();
    if (message.type() === "error" || scriptCsp(text)) {
      errors.push(`${message.type()}: ${text}`);
    }
  });
  page.on("pageerror", (error) => {
    errors.push(error.message);
  });
  return errors;
}

function scriptCsp(text: string): boolean {
  return (
    /content security policy/i.test(text) &&
    /script-src|unsafe-inline|unsafe-eval|refused to execute/i.test(text)
  );
}

async function expectAccessible(page: Page, errors: string[]): Promise<void> {
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (item) => item.impact === "serious" || item.impact === "critical",
  );
  expect(serious, JSON.stringify(serious, null, 2)).toEqual([]);
  expect(errors, errors.join("\n")).toEqual([]);
}

test("ученик отвечает, репетитор видит попытку", async ({ page }) => {
  const errors = watch(page);
  const home = await page.goto(`${stack.origin}/`);
  expect(home?.headers()["content-security-policy"]).toBe(CSP);

  await page.evaluate(async () => {
    const response = await fetch("/auth/dev", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        telegram_id: "tg-demo-student",
        role: "student",
      }),
    });
    if (!response.ok) {
      throw new Error(`${response.status} ${await response.text()}`);
    }
  });

  const assignmentReady = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      url.pathname === "/assignments/asg-demo" &&
      response.request().method() === "GET"
    );
  });
  const task = await page.goto(`${stack.origin}/task/asg-demo/item-a`);
  expect(task?.headers()["content-security-policy"]).toBe(CSP);
  const assignment = await assignmentReady;
  const payload = await assignment.text();
  expect(assignment.ok(), payload).toBeTruthy();
  expect(payload).not.toContain('"key"');
  expect(payload).not.toContain("12");

  await expect(
    page.getByRole("radio", { name: "3. 3", exact: true }),
  ).toBeVisible();
  const before = await page.locator("main").innerText();
  expect(before).not.toContain("12");
  expect(before).not.toContain("[3]");
  expect(before.toLowerCase()).not.toContain("решение");
  await expectAccessible(page, errors);

  await page.getByRole("radio", { name: "3. 3", exact: true }).check();
  await page.getByRole("button", { name: "Ответить" }).click();
  await expect(page.getByText("верно", { exact: true })).toBeVisible();
  const after = await page.locator("main").innerText();
  expect(after).not.toContain("12");
  expect(after.toLowerCase()).not.toContain("решение");
  await expectAccessible(page, errors);

  const browser = page.context().browser();
  if (!browser) {
    throw new Error("browser missing");
  }
  const tutorContext = await browser.newContext({
    viewport: { width: 390, height: 844 },
  });
  const tutor = await tutorContext.newPage();
  const tutorErrors = watch(tutor);
  try {
    const tutorHome = await tutor.goto(`${stack.origin}/tutor`);
    expect(tutorHome?.headers()["content-security-policy"]).toBe(CSP);
    await tutor.getByRole("textbox").fill("tg-demo-tutor");
    await tutor.getByRole("button", { name: "Войти" }).click();
    await tutor.getByRole("button", { name: "student-demo" }).click();
    await expect(tutor.getByText("[3]")).toBeVisible();
    await expect(tutor.getByText("student-other")).toHaveCount(0);
    await expectAccessible(tutor, tutorErrors);
  } finally {
    await tutorContext.close();
  }
});
