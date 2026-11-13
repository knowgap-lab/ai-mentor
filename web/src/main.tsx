import { MantineProvider } from "@mantine/core";
import { StrictMode } from "react";
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
  throw new Error("root element is missing");
}

createRoot(rootElement).render(
  <StrictMode>
    <MantineProvider
      theme={theme}
      forceColorScheme="light"
      withCssVariables={false}
      withGlobalClasses={false}
    >
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </MantineProvider>
  </StrictMode>,
);
