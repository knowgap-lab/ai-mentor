import { MantineProvider } from "@mantine/core";
import type { Preview } from "@storybook/react-vite";
import { initialize, mswLoader } from "msw-storybook-addon";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { theme } from "../src/design/theme";
import { storybookHandlers } from "./msw/handlers";
import "@mantine/core/styles.css";
import "@fontsource-variable/golos-text";
import "../src/design/theme.css";
import "../src/design/design.css";

if (typeof window !== "undefined" && !window.matchMedia) {
  window.matchMedia = (query: string): MediaQueryList =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
      addListener: () => undefined,
      removeListener: () => undefined,
    }) as MediaQueryList;
}

initialize();

function withProviders(Story: () => ReactNode) {
  return (
    <MantineProvider
      theme={theme}
      forceColorScheme="light"
      withCssVariables={false}
      withGlobalClasses={false}
    >
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    </MantineProvider>
  );
}

const preview: Preview = {
  decorators: [withProviders],
  loaders: [mswLoader],
  parameters: {
    layout: "fullscreen",
    controls: { matchers: { color: /(background|color)$/i, date: /Date$/i } },
    a11y: {
      test: "todo",
    },
    msw: {
      handlers: storybookHandlers,
    },
  },
};

export default preview;