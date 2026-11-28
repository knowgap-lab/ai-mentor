import { MantineProvider } from "@mantine/core";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { App } from "./App";
import "@mantine/core/styles.css";
import "@fontsource-variable/golos-text";
import "./design/theme.css";
import "./design/design.css";
import { theme } from "./design/theme";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("no root");
}

createRoot(rootElement).render(
  <MantineProvider
    theme={theme}
    forceColorScheme="light"
    withCssVariables={false}
    withGlobalClasses={false}
  >
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </MantineProvider>,
);
