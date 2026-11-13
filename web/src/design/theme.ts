import {
  createTheme,
  type DefaultMantineColor,
  type MantineColorsTuple,
} from "@mantine/core";

// Тетрадь и ручка (docs/platform/design.md): синие чернила, красная ручка, графит.
const ink: MantineColorsTuple = [
  "#EEF2FB",
  "#DCE3F6",
  "#B6C4EC",
  "#8EA3E1",
  "#6C87D7",
  "#5370CF",
  "#3D5BC4",
  "#2F4AA8",
  "#263D8C",
  "#1B2C66",
];

const gray: MantineColorsTuple = [
  "#F6F7F9",
  "#EEF0F3",
  "#E1E4E9",
  "#CDD2DA",
  "#AEB5C0",
  "#8A92A0",
  "#6A7280",
  "#4E5562",
  "#353A44",
  "#1F2329",
];

const red: MantineColorsTuple = [
  "#FDEEEC",
  "#F9D9D5",
  "#F1B0A8",
  "#E8857A",
  "#DF6255",
  "#D44C3E",
  "#C43D30",
  "#A93226",
  "#8C2A20",
  "#5F1C15",
];

const green: MantineColorsTuple = [
  "#EAF6EF",
  "#D3EDDD",
  "#A7DABA",
  "#78C597",
  "#52B37A",
  "#3AA768",
  "#2B9459",
  "#217A49",
  "#1A623A",
  "#114127",
];

const yellow: MantineColorsTuple = [
  "#FDF6E7",
  "#FAEBC9",
  "#F3D592",
  "#EBBE59",
  "#E3A92C",
  "#DA9A12",
  "#C1850B",
  "#9C6A08",
  "#7A5306",
  "#523704",
];

const TARGET = "2.75rem";
const isLarge = (size?: string) => size === "lg" || size === "xl";

const fontFamily =
  "'Golos Text Variable', system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif";

export const theme = createTheme({
  primaryColor: "ink",
  primaryShade: 7,
  colors: { ink, gray, red, green, yellow },
  white: "#FFFFFF",
  black: "#1F2329",
  fontFamily,
  headings: {
    fontFamily,
    fontWeight: "600",
    sizes: {
      h1: { fontSize: "1.5rem", lineHeight: "1.25" },
      h2: { fontSize: "1.25rem", lineHeight: "1.3" },
      h3: { fontSize: "1.125rem", lineHeight: "1.35" },
    },
  },
  fontSizes: {
    xs: "0.8125rem",
    sm: "0.875rem",
    md: "1rem",
    lg: "1.125rem",
    xl: "1.25rem",
  },
  lineHeights: { md: "1.5" },
  spacing: {
    xs: "0.25rem",
    sm: "0.5rem",
    md: "1rem",
    lg: "1.5rem",
    xl: "2rem",
  },
  radius: {
    xs: "0.25rem",
    sm: "0.375rem",
    md: "0.625rem",
    lg: "0.875rem",
    xl: "1.25rem",
  },
  defaultRadius: "md",
  focusRing: "auto",
  components: {
    // DS-R8: всё нажимаемое не ниже 44 px; крупные размеры lg и xl остаются своими.
    Button: {
      defaultProps: { size: "md" },
      vars: (_theme: unknown, props: { size?: string }) =>
        isLarge(props.size)
          ? { root: {} }
          : { root: { "--button-height": TARGET } },
    },
    ActionIcon: {
      vars: (_theme: unknown, props: { size?: string }) =>
        isLarge(props.size) ? { root: {} } : { root: { "--ai-size": TARGET } },
    },
    Input: {
      vars: (_theme: unknown, props: { size?: string }) =>
        isLarge(props.size)
          ? { wrapper: {} }
          : { wrapper: { "--input-height": TARGET } },
    },
    TextInput: { defaultProps: { size: "md" } },
  },
});

type ExtendedColors = "ink" | DefaultMantineColor;

declare module "@mantine/core" {
  export interface MantineThemeColorsOverride {
    colors: Record<ExtendedColors, MantineColorsTuple>;
  }
}
