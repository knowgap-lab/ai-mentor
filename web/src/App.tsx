import { Text } from "@mantine/core";
import { Link, Route, Routes } from "react-router";
import { TaskPage } from "./task/TaskPage";

export function App() {
  return (
    <Routes>
      <Route path="/task/:assignmentId/:itemId" element={<TaskPage />} />
      <Route path="*" element={<Home />} />
    </Routes>
  );
}

function Home() {
  return (
    <main>
      <Text component="h1">ИИ-ментор</Text>
      <Link to="/next">дальше</Link>
    </main>
  );
}
