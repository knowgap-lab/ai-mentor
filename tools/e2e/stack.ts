import { spawn, execFile, type ChildProcess } from "node:child_process";
import { randomBytes } from "node:crypto";
import { createServer } from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

const seedSQL = `
BEGIN;
INSERT INTO core.app_user (id, role, owner_tutor_id, status) VALUES
  ('tutor-demo', 'tutor', NULL, 'active'),
  ('tutor-other', 'tutor', NULL, 'active'),
  ('student-demo', 'student', 'tutor-demo', 'active'),
  ('student-other', 'student', 'tutor-other', 'active');
INSERT INTO core.user_pii (user_id, telegram_id) VALUES
  ('tutor-demo', 'tg-demo-tutor'),
  ('tutor-other', 'tg-demo-other'),
  ('student-demo', 'tg-demo-student'),
  ('student-other', 'tg-demo-other-student');
INSERT INTO core.assignment (
  id, student_id, state, kind, catalog_version, place, snapshot
) VALUES (
  'asg-demo',
  'student-demo',
  'issued',
  'homework',
  1,
  'home',
  '{"items":[{"item_id":"item-a","part":"A","format":"ct.choice1","key":[3]},{"item_id":"item-b","part":"B","format":"ct.integer","key":12}]}'
);
INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
VALUES
  ('asg-demo', 'item-a', 'open', false, 1),
  ('asg-demo', 'item-b', 'open', false, 2);
COMMIT;
`;

export type Stack = {
  origin: string;
  stop: () => Promise<void>;
};

type CommandResult = {
  stdout: string;
  stderr: string;
};

export async function startStack(): Promise<Stack> {
  const postgres = await startPostgres();
  let server: RunningServer | undefined;
  try {
    await migrate(postgres.dsn);
    await seed(postgres.name);
    await buildWeb();
    server = await startServer(postgres.dsn);
    return {
      origin: server.origin,
      stop: async () => {
        await server?.stop();
        await postgres.stop();
      },
    };
  } catch (error) {
    await server?.stop();
    await postgres.stop();
    throw error;
  }
}

async function startPostgres(): Promise<{
  name: string;
  dsn: string;
  stop: () => Promise<void>;
}> {
  const name = `ai-mentor-e2e-${randomBytes(4).toString("hex")}`;
  const volume = `${name}-data`;
  const stop = async () => {
    await run("docker", ["rm", "-f", name], 30_000).catch(() => undefined);
    await run("docker", ["volume", "rm", "-f", volume], 30_000).catch(
      () => undefined,
    );
  };
  try {
    await run(
      "docker",
      [
        "run",
        "-d",
        "--rm",
        "--pull",
        "never",
        "--name",
        name,
        "-v",
        `${volume}:/var/lib/postgresql/data`,
        "-e",
        "POSTGRES_HOST_AUTH_METHOD=trust",
        "-e",
        "POSTGRES_DB=ai_mentor_test",
        "-p",
        "127.0.0.1::5432",
        "postgres:16.15",
      ],
      40_000,
    );
  } catch (error) {
    await stop();
    throw new Error(`docker run:\n${errorText(error)}`);
  }
  try {
    await waitPostgres(name);
    const port = await publishedPort(name);
    const dsn = `postgres://postgres@127.0.0.1:${port}/ai_mentor_test?sslmode=disable`;
    return { name, dsn, stop };
  } catch (error) {
    await stop();
    throw error;
  }
}

async function waitPostgres(name: string): Promise<void> {
  const deadline = Date.now() + 60_000;
  let last = "";
  while (Date.now() < deadline) {
    try {
      await run(
        "docker",
        ["exec", name, "pg_isready", "-U", "postgres", "-d", "ai_mentor_test"],
        5_000,
      );
      return;
    } catch (error) {
      last = errorText(error);
      await delay(300);
    }
  }
  throw new Error(`postgres not ready:\n${last}`);
}

async function publishedPort(name: string): Promise<string> {
  const deadline = Date.now() + 15_000;
  let last = "";
  while (Date.now() < deadline) {
    try {
      const { stdout } = await run("docker", ["port", name, "5432/tcp"], 5_000);
      for (const line of stdout.split("\n")) {
        const trimmed = line.trim();
        if (trimmed.startsWith("127.0.0.1:")) {
          const port = trimmed.slice("127.0.0.1:".length);
          if (port !== "") {
            return port;
          }
        }
      }
      last = stdout.trim();
    } catch (error) {
      last = errorText(error);
    }
    await delay(200);
  }
  throw new Error(`docker port:\n${last}`);
}

async function migrate(dsn: string): Promise<void> {
  try {
    await run("go", ["run", "./cmd/migrate"], 180_000, {
      cwd: path.join(root, "core"),
      env: { ...process.env, DATABASE_URL: dsn },
    });
  } catch (error) {
    throw new Error(`migrate:\n${errorText(error)}`);
  }
}

async function seed(name: string): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const child = spawn(
      "docker",
      [
        "exec",
        "-i",
        name,
        "psql",
        "-U",
        "postgres",
        "-d",
        "ai_mentor_test",
        "-v",
        "ON_ERROR_STOP=1",
      ],
      { stdio: ["pipe", "pipe", "pipe"] },
    );
    let stderr = "";
    let stdout = "";
    child.stdout.on("data", (chunk: Buffer) => {
      stdout += chunk.toString();
    });
    child.stderr.on("data", (chunk: Buffer) => {
      stderr += chunk.toString();
    });
    child.on("error", (error) => {
      reject(new Error(`psql:\n${error.message}`));
    });
    child.on("close", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`psql:\n${stderr || stdout}`));
    });
    child.stdin.end(seedSQL);
  });
}

async function buildWeb(): Promise<void> {
  const cwd = path.join(root, "web");
  try {
    await run("npm", ["install", "--ignore-scripts"], 180_000, { cwd });
    await run("npm", ["run", "build"], 180_000, { cwd });
  } catch (error) {
    throw new Error(`web build:\n${errorText(error)}`);
  }
}

type RunningServer = {
  origin: string;
  stop: () => Promise<void>;
};

async function startServer(dsn: string): Promise<RunningServer> {
  const port = await freePort();
  const origin = `http://127.0.0.1:${port}`;
  const child = spawn("go", ["run", "./cmd/server"], {
    cwd: path.join(root, "core"),
    detached: true,
    env: {
      ...process.env,
      ENV: "local",
      AUTH_DEV_MODE: "true",
      DATABASE_URL: dsn,
      APP_ORIGIN: origin,
      ADDR: `127.0.0.1:${port}`,
      WEB_DIST: path.join(root, "web", "dist"),
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  let logs = "";
  const append = (chunk: Buffer) => {
    logs += chunk.toString();
    if (logs.length > 20_000) {
      logs = logs.slice(-20_000);
    }
  };
  child.stdout?.on("data", append);
  child.stderr?.on("data", append);
  let exited = false;
  child.on("exit", () => {
    exited = true;
  });
  const stop = () => stopProcess(child);
  try {
    await waitHealth(origin, () => exited, () => logs);
  } catch (error) {
    await stop();
    throw new Error(`${errorText(error)}\n${logs}`);
  }
  return { origin, stop };
}

async function waitHealth(
  origin: string,
  exited: () => boolean,
  logs: () => string,
): Promise<void> {
  const deadline = Date.now() + 120_000;
  let last = "";
  while (Date.now() < deadline) {
    if (exited()) {
      throw new Error(`server exited:\n${logs()}`);
    }
    try {
      const response = await fetch(`${origin}/healthz`);
      const body = await response.text();
      if (response.ok && body === "ok") {
        return;
      }
      last = `status ${response.status} ${body}`;
    } catch (error) {
      last = errorText(error);
    }
    await delay(200);
  }
  throw new Error(`server not ready: ${last}\n${logs()}`);
}

function stopProcess(child: ChildProcess): Promise<void> {
  return new Promise((resolve) => {
    if (child.pid === undefined) {
      resolve();
      return;
    }
    let settled = false;
    const finish = () => {
      if (settled) {
        return;
      }
      settled = true;
      resolve();
    };
    child.once("exit", finish);
    try {
      process.kill(-child.pid, "SIGTERM");
    } catch {
      try {
        child.kill("SIGTERM");
      } catch {
        finish();
        return;
      }
    }
    setTimeout(() => {
      if (child.pid !== undefined) {
        try {
          process.kill(-child.pid, "SIGKILL");
        } catch {
          // Процесс уже завершился.
        }
      }
      finish();
    }, 5_000).unref();
  });
}

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (address === null || typeof address === "string") {
        server.close();
        reject(new Error("no free port"));
        return;
      }
      const { port } = address;
      server.close((error) => {
        if (error) {
          reject(error);
          return;
        }
        resolve(port);
      });
    });
  });
}

async function run(
  command: string,
  args: string[],
  timeout: number,
  options?: { cwd?: string; env?: NodeJS.ProcessEnv },
): Promise<CommandResult> {
  try {
    const { stdout, stderr } = await execFileAsync(command, args, {
      timeout,
      cwd: options?.cwd,
      env: options?.env,
      maxBuffer: 10 * 1024 * 1024,
    });
    return { stdout: String(stdout), stderr: String(stderr) };
  } catch (error) {
    const err = error as {
      stdout?: unknown;
      stderr?: unknown;
      message?: string;
    };
    const text = [err.stderr, err.stdout, err.message]
      .map((part) => (part === undefined ? "" : String(part).trim()))
      .filter((part) => part !== "")
      .join("\n");
    throw new Error(text);
  }
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}
