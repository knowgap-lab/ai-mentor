// Демо-данные домашней репетитора (синтетика; без связи с приватным каталогом).

export type TutorHomeStudent = {
  id: string;
  displayName: string;
  icon: string;
  homeworkDone: number;
  homeworkTotal: number;
  coursePercent: number;
};

export type TutorHomeProfile = {
  displayName: string;
  icon: string;
  about: string;
};

export const TUTOR_HOME_PROFILE: TutorHomeProfile = {
  displayName: "Марина Волкова",
  icon: "🎓",
  about:
    "Репетитор по математике. Готовлю к ЦТ и ЦЭ: план, домашние задания и разбор ошибок в одном кабинете.",
};

export const TUTOR_HOME_STUDENTS: TutorHomeStudent[] = [
  {
    id: "demo-student-01",
    displayName: "Алина К.",
    icon: "🦊",
    homeworkDone: 6,
    homeworkTotal: 8,
    coursePercent: 58,
  },
  {
    id: "demo-student-02",
    displayName: "Богдан М.",
    icon: "🐻",
    homeworkDone: 3,
    homeworkTotal: 5,
    coursePercent: 41,
  },
  {
    id: "demo-student-03",
    displayName: "Вера С.",
    icon: "🐰",
    homeworkDone: 8,
    homeworkTotal: 8,
    coursePercent: 72,
  },
  {
    id: "demo-student-04",
    displayName: "Глеб Т.",
    icon: "🐺",
    homeworkDone: 1,
    homeworkTotal: 6,
    coursePercent: 22,
  },
  {
    id: "demo-student-05",
    displayName: "Дина Л.",
    icon: "🦉",
    homeworkDone: 4,
    homeworkTotal: 7,
    coursePercent: 49,
  },
  {
    id: "demo-student-06",
    displayName: "Егор П.",
    icon: "🐧",
    homeworkDone: 5,
    homeworkTotal: 5,
    coursePercent: 63,
  },
];

export const TUTOR_HOME_SOCIAL = [
  { id: "tg", label: "Telegram", href: "#" },
  { id: "vk", label: "VK", href: "#" },
  { id: "site", label: "Сайт", href: "#" },
] as const;
