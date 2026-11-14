import { Button } from "@mantine/core";
import type { ReactNode } from "react";
import { Link } from "react-router";
import classes from "./ui.module.css";

// Каркас экрана (DS-R10): заголовок, «назад», ширина до 640 px, место под ActionBar.
export function Screen({
  title,
  backTo,
  withActionBar = false,
  children,
}: {
  title: string;
  backTo?: string;
  withActionBar?: boolean;
  children: ReactNode;
}) {
  return (
    <main
      className={
        withActionBar
          ? `${classes.screen} ${classes.screenWithBar}`
          : classes.screen
      }
    >
      <header className={classes.header}>
        {backTo ? (
          <Link to={backTo} className={classes.back} aria-label="Назад">
            ←
          </Link>
        ) : null}
        <h1 className={classes.title}>{title}</h1>
      </header>
      {children}
    </main>
  );
}

export function List({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <ul className={classes.list} aria-label={label}>
      {children}
    </ul>
  );
}

// Строка списка: вся строка — одна ссылка (DS-R11).
export function ListRow({
  to,
  title,
  subtitle,
  aside,
}: {
  to: string;
  title: string;
  subtitle?: string;
  aside?: ReactNode;
}) {
  return (
    <li className={classes.row}>
      <Link to={to} className={classes.rowLink}>
        <span className={classes.rowText}>
          <span className={classes.rowTitle}>{title}</span>
          {subtitle ? (
            <>
              <br />
              <span className={classes.rowSub}>{subtitle}</span>
            </>
          ) : null}
        </span>
        {aside}
      </Link>
    </li>
  );
}

// Строка списка без маршрута: вся строка — одна кнопка (DS-R11).
export function ListButton({
  title,
  subtitle,
  aside,
  onClick,
}: {
  title: string;
  subtitle?: string;
  aside?: ReactNode;
  onClick: () => void;
}) {
  return (
    <li className={classes.row}>
      <button
        type="button"
        className={`${classes.rowLink} ${classes.rowButton}`}
        onClick={onClick}
      >
        <span className={classes.rowText}>
          <span className={classes.rowTitle}>{title}</span>
          {subtitle ? (
            <>
              <br />
              <span className={classes.rowSub}>{subtitle}</span>
            </>
          ) : null}
        </span>
        {aside}
      </button>
    </li>
  );
}

// Строка варианта ответа: зона касания не ниже 44 px (DS-R8).
export function ChoiceRow({ children }: { children: ReactNode }) {
  return <div className={classes.choice}>{children}</div>;
}

// Лист задачи в клетку.
export function TaskSheet({
  label,
  children,
}: {
  label?: string;
  children: ReactNode;
}) {
  return (
    <section className={classes.sheet} aria-label={label ?? "Задача"}>
      {label ? <p className={classes.sheetLabel}>{label}</p> : null}
      {children}
    </section>
  );
}

// Нижняя панель: одно главное действие и не больше одного второстепенного (DS-R10).
export function ActionBar({
  primary,
  secondary,
}: {
  primary: {
    label: string;
    onClick: () => void;
    disabled?: boolean;
    loading?: boolean;
  };
  secondary?: { label: string; onClick: () => void; disabled?: boolean };
}) {
  return (
    <div className={classes.actionBar}>
      <div className={classes.actionBarInner}>
        {secondary ? (
          <Button
            variant="default"
            onClick={secondary.onClick}
            disabled={secondary.disabled}
          >
            {secondary.label}
          </Button>
        ) : null}
        <Button
          onClick={primary.onClick}
          disabled={primary.disabled}
          loading={primary.loading}
        >
          {primary.label}
        </Button>
      </div>
    </div>
  );
}
