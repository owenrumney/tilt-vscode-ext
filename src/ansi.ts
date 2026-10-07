export interface AnsiSpan {
  text: string;
  /** CSS colour: a theme variable for the 16 ANSI colours, hex otherwise. */
  color?: string;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  dim?: boolean;
}

interface Style {
  color?: string;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  dim?: boolean;
}

const NAMES = [
  "Black",
  "Red",
  "Green",
  "Yellow",
  "Blue",
  "Magenta",
  "Cyan",
  "White",
];

// OSC (hyperlinks, titles) carries no colour, so it goes before anything else.
const OSC = /\u001b\][\s\S]*?(?:\u0007|\u001b\\)/g;
// Private parameter bytes (?<>=!) and intermediates ( -/) are part of the
// grammar: spinners and progress bars emit ESC[?25l, cursor styles ESC[0 q.
const CSI = /\u001b\[([0-9;?<>=!]*)([ -/]*)([@-~])/g;
// Short escapes such as ESC(B (charset select). The final byte excludes "[",
// so this never eats a CSI.
const ESC2 = /\u001b[ -/]*[0-Z\\-~]/g;

/** Splits a line into styled runs, dropping escape sequences that are not colour. */
export function parseAnsi(text: string): AnsiSpan[] {
  const clean = text.replace(OSC, "").replace(ESC2, "");
  const spans: AnsiSpan[] = [];
  let style: Style = {};
  let last = 0;

  CSI.lastIndex = 0;
  for (let m = CSI.exec(clean); m; m = CSI.exec(clean)) {
    push(spans, clean.slice(last, m.index), style);
    last = CSI.lastIndex;
    // Only a plain numeric SGR carries colour; private/intermediate forms do not.
    if (m[3] === "m" && !m[2] && !/[?<>=!]/.test(m[1])) {
      style = applySgr(style, m[1]);
    }
  }
  push(spans, clean.slice(last), style);
  return spans;
}

export function stripAnsi(text: string): string {
  return text.replace(OSC, "").replace(ESC2, "").replace(CSI, "");
}

function push(spans: AnsiSpan[], text: string, style: Style): void {
  if (text) {
    spans.push({ text, ...style });
  }
}

function applySgr(style: Style, params: string): Style {
  const codes = (params || "0").split(";").map((p) => Number(p || "0"));
  let next: Style = { ...style };

  for (let i = 0; i < codes.length; i++) {
    const code = codes[i];
    if (code === 0) {
      next = {};
    } else if (code === 1) {
      next.bold = true;
    } else if (code === 2) {
      next.dim = true;
    } else if (code === 3) {
      next.italic = true;
    } else if (code === 4) {
      next.underline = true;
    } else if (code === 22) {
      next.bold = undefined;
      next.dim = undefined;
    } else if (code === 23) {
      next.italic = undefined;
    } else if (code === 24) {
      next.underline = undefined;
    } else if (code === 39) {
      next.color = undefined;
    } else if (code >= 30 && code <= 37) {
      next.color = ansiVar(NAMES[code - 30]);
    } else if (code >= 90 && code <= 97) {
      next.color = ansiVar(`Bright${NAMES[code - 90]}`);
    } else if (code === 38) {
      const [color, used] = extended(codes, i);
      next.color = color ?? next.color;
      i += used;
    } else if (code === 48) {
      // Background colours are skipped, so only the index is consumed.
      i += extended(codes, i)[1];
    }
  }
  return next;
}

function extended(codes: number[], i: number): [string | undefined, number] {
  if (codes[i + 1] === 5) {
    return [palette(codes[i + 2]), 2];
  }
  if (codes[i + 1] === 2) {
    return [rgb(codes[i + 2], codes[i + 3], codes[i + 4]), 4];
  }
  return [undefined, 0];
}

function palette(n: number): string | undefined {
  if (n === undefined || n < 0 || n > 255) {
    return undefined;
  }
  if (n < 8) {
    return ansiVar(NAMES[n]);
  }
  if (n < 16) {
    return ansiVar(`Bright${NAMES[n - 8]}`);
  }
  if (n < 232) {
    const c = n - 16;
    const level = (v: number) => (v === 0 ? 0 : 55 + v * 40);
    return rgb(level(Math.floor(c / 36)), level(Math.floor(c / 6) % 6), level(c % 6));
  }
  const grey = 8 + (n - 232) * 10;
  return rgb(grey, grey, grey);
}

function rgb(r?: number, g?: number, b?: number): string | undefined {
  if ([r, g, b].some((v) => v === undefined || v < 0 || v > 255)) {
    return undefined;
  }
  const hex = (v = 0) => v.toString(16).padStart(2, "0");
  return `#${hex(r)}${hex(g)}${hex(b)}`;
}

function ansiVar(name: string): string {
  return `var(--vscode-terminal-ansi${name})`;
}
