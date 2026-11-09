import DOMPurify from "dompurify";
import { createElement, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import rehypeKatex from "rehype-katex";
import remarkMath from "remark-math";
import "katex/dist/katex.min.css";

const KATEX_OPTIONS = { trust: false } as const;

const SVG_TAGS = new Set([
  "svg",
  "g",
  "path",
  "circle",
  "rect",
  "line",
  "polyline",
  "polygon",
  "text",
  "tspan",
  "title",
  "defs",
  "ellipse",
]);

const SVG_ATTRS = new Set([
  "d",
  "fill",
  "stroke",
  "viewBox",
  "xmlns",
  "cx",
  "cy",
  "r",
  "rx",
  "ry",
  "x",
  "y",
  "x1",
  "y1",
  "x2",
  "y2",
  "width",
  "height",
  "points",
  "transform",
  "stroke-width",
  "stroke-linecap",
  "stroke-linejoin",
  "opacity",
  "fill-opacity",
  "stroke-opacity",
]);

function purifier() {
  return DOMPurify(window);
}

function sanitize(
  source: string,
  config: Parameters<ReturnType<typeof purifier>["sanitize"]>[1],
): string {
  const engine = purifier();
  if (!engine.isSupported) {
    return source.replace(/<[^>]*>/g, "");
  }
  const clean = engine.sanitize(source, config);
  return typeof clean === "string" ? clean : String(clean);
}

function protectMath(source: string): { text: string; math: string[] } {
  const math: string[] = [];
  const text = source.replace(/\$\$[\s\S]*?\$\$|\$[^$\n]*?\$/g, (part) => {
    const token = `MATHTOKEN${math.length}MATHTOKEN`;
    math.push(part);
    return token;
  });
  return { text, math };
}

function restoreMath(source: string, math: string[]): string {
  return source.replace(/MATHTOKEN(\d+)MATHTOKEN/g, (_match, index: string) => {
    return math[Number(index)] ?? "";
  });
}

function prepareStatement(source: string): {
  markdown: string;
  svgs: string[];
} {
  const protectedMath = protectMath(source);
  const svgs: string[] = [];
  const withoutSvg = protectedMath.text.replace(
    /<svg\b[^>]*>[\s\S]*?<\/svg>/gi,
    (part) => {
      const clean = sanitize(part, {
        USE_PROFILES: { svg: true, svgFilters: true },
        FORBID_TAGS: [
          "script",
          "style",
          "foreignObject",
          "iframe",
          "object",
          "embed",
        ],
        FORBID_ATTR: ["href", "xlink:href", "style"],
      });
      if (clean.toLowerCase().includes("<svg")) {
        svgs.push(clean);
      }
      return "";
    },
  );
  const stripped = sanitize(withoutSvg, {
    ALLOWED_TAGS: [],
    ALLOWED_ATTR: [],
    KEEP_CONTENT: true,
  });
  return { markdown: restoreMath(stripped, protectedMath.math), svgs };
}

function reactAttr(name: string): string | null {
  if (name.toLowerCase().startsWith("on") || !SVG_ATTRS.has(name)) {
    return null;
  }
  if (name === "viewBox") {
    return "viewBox";
  }
  return name.replace(/-([a-z])/g, (_match, letter: string) =>
    letter.toUpperCase(),
  );
}

function renderSvgNode(node: ChildNode): ReactNode {
  if (node.nodeType === Node.TEXT_NODE) {
    return node.textContent;
  }
  if (!(node instanceof Element)) {
    return null;
  }
  const tag = node.tagName.toLowerCase();
  if (!SVG_TAGS.has(tag)) {
    return null;
  }
  const props: Record<string, string> = {};
  for (const attr of node.attributes) {
    const name = reactAttr(attr.name);
    if (name) {
      props[name] = attr.value;
    }
  }
  const children = Array.from(node.childNodes).map((child) =>
    renderSvgNode(child),
  );
  return createElement(tag, props, ...children);
}

function SanitizedSvg({ markup }: { markup: string }) {
  const doc = new DOMParser().parseFromString(markup, "image/svg+xml");
  if (doc.querySelector("parsererror")) {
    return null;
  }
  return renderSvgNode(doc.documentElement);
}

export function Statement({ source }: { source: string }) {
  const { markdown, svgs } = prepareStatement(source);
  return (
    <div>
      <ReactMarkdown
        skipHtml
        remarkPlugins={[remarkMath]}
        rehypePlugins={[[rehypeKatex, KATEX_OPTIONS]]}
      >
        {markdown}
      </ReactMarkdown>
      {svgs.map((markup, index) => (
        <SanitizedSvg key={index} markup={markup} />
      ))}
    </div>
  );
}
