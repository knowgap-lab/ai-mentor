import { Progress, Stack, Switch, Text } from "@mantine/core";
import { useCallback, useState } from "react";
import { Link } from "react-router";
import {
  TUTOR_HOME_PROFILE,
  TUTOR_HOME_SOCIAL,
  TUTOR_HOME_STUDENTS,
  type TutorHomeStudent,
} from "./tutor-home-data";
import classes from "./tutor-home.module.css";

const SCHEME_KEY = "am-tutor-home-scheme";

type ColorScheme = "light" | "dark";

function readStoredScheme(): ColorScheme {
  if (typeof window === "undefined") {
    return "light";
  }
  const stored = window.localStorage.getItem(SCHEME_KEY);
  return stored === "dark" ? "dark" : "light";
}

function persistScheme(scheme: ColorScheme): void {
  try {
    window.localStorage.setItem(SCHEME_KEY, scheme);
  } catch {
    // частный режим или запрет storage — не мешаем экрану
  }
}

function StudentCard({ student }: { student: TutorHomeStudent }) {
  const homeworkPercent =
    student.homeworkTotal > 0
      ? Math.round((student.homeworkDone / student.homeworkTotal) * 100)
      : 0;

  return (
    <li className={classes.studentCard}>
      <div className={classes.studentHead}>
        <span className={classes.studentIcon} aria-hidden="true">
          {student.icon}
        </span>
        <p className={classes.studentName}>{student.displayName}</p>
      </div>
      <Stack gap={4}>
        <Text className={classes.metricLabel}>Домашнее задание</Text>
        <Text className={`${classes.metricValue} am-num`}>
          {student.homeworkDone} из {student.homeworkTotal}
        </Text>
        <Progress
          value={homeworkPercent}
          aria-label={`Домашнее задание: ${student.homeworkDone} из ${student.homeworkTotal}`}
        />
      </Stack>
      <Stack gap={4}>
        <Text className={classes.metricLabel}>Курс</Text>
        <Text className={`${classes.metricValue} am-num`}>
          {student.coursePercent}%
        </Text>
        <Progress
          value={student.coursePercent}
          aria-label={`Курс: ${student.coursePercent} процентов`}
        />
      </Stack>
    </li>
  );
}

export function TutorHomePage() {
  const [scheme, setScheme] = useState<ColorScheme>(readStoredScheme);

  const onSchemeChange = useCallback((checked: boolean) => {
    const next: ColorScheme = checked ? "dark" : "light";
    setScheme(next);
    persistScheme(next);
  }, []);

  return (
    <div
      className={classes.root}
      data-am-scheme={scheme}
      data-testid="tutor-home-root"
    >
      <div className={classes.shell}>
        <div className={classes.topRow}>
          <h1 className={classes.pageTitle}>Домашняя репетитора</h1>
          <Switch
            checked={scheme === "dark"}
            onChange={(event) => onSchemeChange(event.currentTarget.checked)}
            label={scheme === "dark" ? "Тёмная тема" : "Светлая тема"}
            aria-label="Переключить светлую и тёмную тему"
          />
        </div>

        <section
          aria-label="Профиль репетитора"
          className={classes.profileCard}
        >
          <span className={classes.profileIcon} aria-hidden="true">
            {TUTOR_HOME_PROFILE.icon}
          </span>
          <div>
            <p className={classes.profileName}>
              {TUTOR_HOME_PROFILE.displayName}
            </p>
            <p className={classes.profileAbout}>{TUTOR_HOME_PROFILE.about}</p>
          </div>
        </section>

        <section aria-labelledby="tutor-students-heading">
          <h2 id="tutor-students-heading" className={classes.sectionTitle}>
            Ученики
          </h2>
          <ul className={classes.board} aria-label="Синтетические ученики">
            {TUTOR_HOME_STUDENTS.map((student) => (
              <StudentCard key={student.id} student={student} />
            ))}
          </ul>
        </section>

        <footer className={classes.footer}>
          <Text size="sm" c="dimmed" mb="sm">
            Соцсети (заглушки)
          </Text>
          <ul className={classes.socialList}>
            {TUTOR_HOME_SOCIAL.map((item) => (
              <li key={item.id}>
                <a
                  className={classes.socialLink}
                  href={item.href}
                  onClick={(event) => event.preventDefault()}
                >
                  {item.label}
                </a>
              </li>
            ))}
          </ul>
          <Link className={classes.cabinetLink} to="/tutor">
            Войти в кабинет репетитора
          </Link>
        </footer>
      </div>
    </div>
  );
}
