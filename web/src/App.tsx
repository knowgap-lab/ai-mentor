import { Text } from "@mantine/core";
import { Link } from "react-router";

export function App() {
  return (
    <main>
      <Text component="h1">ИИ-ментор</Text>
      <Link to="/next">дальше</Link>
    </main>
  );
}
