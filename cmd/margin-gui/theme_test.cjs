const { test } = require("node:test");
const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
test("light syntax and filled controls keep readable contrast", () => {
  const css = readFileSync(`${__dirname}/assets/style.css`, "utf8");
  const light = css.split(':root[data-theme="light"] {')[1].split("}")[0];
  const colors = Object.fromEntries([...light.matchAll(/--([\w-]+):\s*([^;]+);/g)].map(([, key, value]) => [key, value.trim()]));
  const color = (role) => {
    const value = colors[role];
    assert.ok(value, `Missing palette role: ${role}`);
    if (value.startsWith("var(--")) return color(value.slice(6, -1));
    if (value.startsWith("#")) return value.slice(1).match(/../g).map((c) => parseInt(c, 16));
    assert.match(value, /^rgba\([\d.,\s]+\)$/);
    return value.slice(5, -1).split(",").map(Number);
  };
  const luminance = (rgb) => {
    const [r, g, b] = rgb.map((c) => c / 255).map((c) => c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const readable = (foreground, ...layers) => {
    const background = layers.reduce((bg, role) => {
      const [r, g, b, alpha = 1] = color(role);
      return [r, g, b].map((c, i) => c * alpha + bg[i] * (1 - alpha));
    }, [255, 255, 255]);
    const [lo, hi] = [luminance(color(foreground)), luminance(background)].sort((a, b) => a - b);
    assert.ok((hi + 0.05) / (lo + 0.05) >= 4.5, `${foreground} on ${layers.join(" + ")} needs at least 4.5:1 contrast`);
  };
  for (const role of Object.keys(colors).filter((key) => key.startsWith("cm-") || key.startsWith("syntax-"))) {
    readable(role, "bg-1");
    readable(role, "bg-1", "chg-bg", "add-hl", "search-bg");
    readable(role, "bg-1", "chg-bg", "search-bg", "add-hl");
  }
  readable("fg", "bg-1", "chg-bg", "del-bg", "del-hl", "search-bg");
  readable("fg-dim", "bg-3");
  readable("fg-faint", "bg-3");
  readable("cursor-ink", "bg-1", "cursor-fill");
  readable("on-primary", "primary-bg");
  readable("on-primary", "primary-hover");
});
