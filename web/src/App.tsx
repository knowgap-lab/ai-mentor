import { useLayoutEffect } from "react";
import { Route, Routes } from "react-router";
import { AssignmentPage } from "./student/AssignmentPage";
import { HomePage } from "./student/HomePage";
import { LessonPage } from "./student/LessonPage";
import { ProfilePage } from "./student/ProfilePage";
import { SummaryPage } from "./student/SummaryPage";
import { TaskPage } from "./task/TaskPage";
import { HomeworkDraftPage } from "./tutor/HomeworkDraftPage";
import { HomeworkResultsPage } from "./tutor/HomeworkResultsPage";
import { NewStudentPage } from "./tutor/NewStudentPage";
import { PreparationPage } from "./tutor/PreparationPage";
import { SessionsPage } from "./tutor/SessionsPage";
import { StudentCardPage } from "./tutor/StudentCardPage";
import { TutorLessonPage } from "./tutor/TutorLessonPage";
import { TutorPage } from "./tutor/TutorPage";
import { DesignPage } from "./design/DesignPage";

function parkProviderStyles(): void {
  const parked = new Set(
    [...document.head.querySelectorAll("style")].map(
      (node) => node.textContent ?? "",
    ),
  );
  for (const node of document.body.querySelectorAll("style")) {
    const text = node.textContent ?? "";
    if (text === "") {
      continue;
    }
    if (!parked.has(text)) {
      const copy = document.createElement("style");
      copy.textContent = text;
      document.head.appendChild(copy);
      parked.add(text);
    }
    node.textContent = "";
  }
}

export function App() {
  useLayoutEffect(() => {
    parkProviderStyles();
  });

  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route path="/lesson/:lessonId" element={<LessonPage />} />
      <Route
        path="/assignment/:assignmentId/summary"
        element={<SummaryPage />}
      />
      <Route path="/assignment/:assignmentId" element={<AssignmentPage />} />
      <Route path="/profile" element={<ProfilePage />} />
      <Route path="/task/:assignmentId/:itemId" element={<TaskPage />} />
      <Route path="/tutor" element={<TutorPage />} />
      <Route path="/tutor/sessions" element={<SessionsPage />} />
      <Route path="/tutor/students/new" element={<NewStudentPage />} />
      <Route
        path="/tutor/students/:studentId/assignments/:assignmentId"
        element={<HomeworkResultsPage />}
      />
      <Route path="/tutor/students/:studentId" element={<StudentCardPage />} />
      <Route
        path="/tutor/lessons/:lessonId/preparation"
        element={<PreparationPage />}
      />
      <Route
        path="/tutor/lessons/:lessonId/homework"
        element={<HomeworkDraftPage />}
      />
      <Route path="/tutor/lessons/:lessonId" element={<TutorLessonPage />} />
      {import.meta.env.DEV ? (
        <Route path="/design" element={<DesignPage />} />
      ) : null}
      <Route path="*" element={<HomePage />} />
    </Routes>
  );
}
