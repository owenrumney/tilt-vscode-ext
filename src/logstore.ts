import { AnsiSpan, parseAnsi, stripAnsi } from "./ansi";
import { RoutedSegment } from "./model";

export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogLine {
  /** Buffer the line belongs to: a resource name, or TILT_KEY. */
  key: string;
  level: LogLevel;
  /** Escape codes stripped, so filters match what the reader sees. */
  text: string;
  spans: AnsiSpan[];
}

export interface LogFilter {
  minLevel: LogLevel;
  text?: string;
}

/** Buffer key for Tilt's own log, which has no resource. */
export const TILT_KEY = "(tilt)";

export const LEVELS: LogLevel[] = ["debug", "info", "warn", "error"];

const SEVERITY: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
};

const MAX_LINES = 5000;

const ZERO: Record<LogLevel, number> = { debug: 0, info: 0, warn: 0, error: 0 };

/** Per-resource ring buffers. Tilt splits chunks mid-line, so a partial tail
 * is held back until its newline. */
export class LogStore {
  private buffers = new Map<string, LogLine[]>();
  private partial = new Map<string, { level: LogLevel; raw: string }>();
  // Running totals, so counts() does not walk the buffer on every view message.
  private tallies = new Map<string, Record<LogLevel, number>>();

  /** Appends segments and returns the lines that completed. */
  append(segments: RoutedSegment[]): LogLine[] {
    const completed: LogLine[] = [];
    for (const segment of segments) {
      const key = segment.resource ?? TILT_KEY;
      const level = normalizeLevel(segment.level);
      const head = this.partial.get(key);
      const parts = ((head?.raw ?? "") + segment.text).split("\n");
      this.partial.delete(key);

      const tail = parts.pop() ?? "";
      for (const raw of parts) {
        const line = toLine(key, level, raw);
        this.push(line);
        completed.push(line);
      }
      if (tail) {
        this.partial.set(key, { level, raw: tail });
      }
    }
    return completed;
  }

  lines(key: string): LogLine[] {
    const buffer = this.buffers.get(key) ?? [];
    const tail = this.partial.get(key);
    return tail ? [...buffer, toLine(key, tail.level, tail.raw)] : [...buffer];
  }

  counts(key: string): Record<LogLevel, number> {
    const counts = { ...(this.tallies.get(key) ?? ZERO) };
    const tail = this.partial.get(key);
    if (tail) {
      counts[tail.level] += 1;
    }
    return counts;
  }

  clear(): void {
    this.buffers.clear();
    this.partial.clear();
    this.tallies.clear();
  }

  private push(line: LogLine): void {
    const buffer = this.buffers.get(line.key) ?? [];
    const tally = this.tallies.get(line.key) ?? { ...ZERO };
    buffer.push(line);
    tally[line.level] += 1;
    if (buffer.length > MAX_LINES) {
      for (const dropped of buffer.splice(0, buffer.length - MAX_LINES)) {
        tally[dropped.level] -= 1;
      }
    }
    this.buffers.set(line.key, buffer);
    this.tallies.set(line.key, tally);
  }
}

function toLine(key: string, level: LogLevel, raw: string): LogLine {
  // Trailing \r would render as a blank glyph in the webview.
  const text = stripAnsi(raw).replace(/\r$/, "");
  return { key, level, text, spans: parseAnsi(raw.replace(/\r$/, "")) };
}

export function normalizeLevel(level?: string): LogLevel {
  switch ((level ?? "").toUpperCase()) {
    case "FATAL":
    case "ERROR":
      return "error";
    case "WARN":
    case "WARNING":
      return "warn";
    case "DEBUG":
    case "VERBOSE":
    case "TRACE":
      return "debug";
    default:
      return "info";
  }
}

/** Builds a predicate from a filter. `/re/` is a regex, anything else a substring. */
export function lineFilter(filter: LogFilter): (line: LogLine) => boolean {
  const min = SEVERITY[filter.minLevel] ?? 0;
  const text = (filter.text ?? "").trim();
  const match = textMatcher(text);
  return (line) => SEVERITY[line.level] >= min && match(line.text);
}

function textMatcher(text: string): (s: string) => boolean {
  if (!text) {
    return () => true;
  }
  if (text.length > 2 && text.startsWith("/") && text.endsWith("/")) {
    try {
      const re = new RegExp(text.slice(1, -1), "i");
      return (s) => re.test(s);
    } catch {
      // An unfinished regex should not blank the log.
      return () => true;
    }
  }
  const needle = text.toLowerCase();
  return (s) => s.toLowerCase().includes(needle);
}
