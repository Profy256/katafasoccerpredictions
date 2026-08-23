/**
 * Best-effort parsing of a pasted block of text into tip rows — one line, one
 * match. There is no fixed format to lean on here: admins paste whatever
 * shape their notes are already in. This never writes anything on its own;
 * it only produces a first guess that BulkAddTipsForm shows back as an
 * editable table, so a wrong guess is always caught before it becomes a tip.
 */

export interface ParsedTip {
  fixtureLabel: string;
  marketLabel: string;
  selectionLabel: string;
  odds: string;
  /** Empty string when no confident date+time was found in the line. */
  kickoffAt: string;
}

const MONTHS = [
  'jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec',
];

/** Odds are always written with a decimal point ("1.85") — dates almost never are. */
const ODDS_RE = /@?\s*(\d{1,2}\.\d{1,3})\b/g;

function findOdds(line: string): { odds: string; rest: string } {
  const matches = [...line.matchAll(ODDS_RE)];
  if (matches.length === 0) return { odds: '', rest: line };
  // The odds are usually the pick's price, stated once near the end of the
  // line — take the last decimal number rather than the first.
  const m = matches[matches.length - 1];
  const rest = line.slice(0, m.index) + line.slice(m.index! + m[0].length);
  return { odds: m[1], rest };
}

function pad2(n: number): string {
  return String(n).padStart(2, '0');
}

/** Turns a detected {year, month, day, hour, minute} into a datetime-local value. */
function toLocalInput(y: number, mo: number, d: number, h: number, mi: number): string {
  return `${y}-${pad2(mo)}-${pad2(d)}T${pad2(h)}:${pad2(mi)}`;
}

function findKickoff(line: string): { kickoffAt: string; rest: string } {
  const now = new Date();

  // ISO-ish: 2026-08-22 15:00 or 2026-08-22T15:00
  let m = /\b(\d{4})-(\d{1,2})-(\d{1,2})(?:[ T](\d{1,2}):(\d{2}))?\b/.exec(line);
  if (m) {
    const [, y, mo, d, h, mi] = m;
    if (h && mi) {
      return {
        kickoffAt: toLocalInput(+y, +mo, +d, +h, +mi),
        rest: line.slice(0, m.index) + line.slice(m.index + m[0].length),
      };
    }
  }

  // Slash: 22/08/2026 15:00 or 22/08 15:00 (day/month, matching en-GB usage
  // elsewhere in this app)
  m = /\b(\d{1,2})\/(\d{1,2})(?:\/(\d{2,4}))?\s+(\d{1,2}):(\d{2})\b/.exec(line);
  if (m) {
    const [, d, mo, y, h, mi] = m;
    const year = y ? (y.length === 2 ? 2000 + +y : +y) : now.getFullYear();
    return {
      kickoffAt: toLocalInput(year, +mo, +d, +h, +mi),
      rest: line.slice(0, m.index) + line.slice(m.index + m[0].length),
    };
  }

  // "22 Aug" / "22 Aug 2026", optionally followed by a time
  const monthPattern = MONTHS.join('|');
  m = new RegExp(
    `\\b(\\d{1,2})\\s+(${monthPattern})[a-z]*(?:\\s+(\\d{4}))?(?:\\s+(\\d{1,2}):(\\d{2}))?\\b`, 'i',
  ).exec(line);
  if (m) {
    const [, d, monAbbr, y, h, mi] = m;
    if (h && mi) {
      const month = MONTHS.indexOf(monAbbr.toLowerCase()) + 1;
      let year = y ? +y : now.getFullYear();
      // No year given and the date has already passed this year — assume
      // it's next year's fixture rather than a stale one.
      if (!y) {
        const guess = new Date(year, month - 1, +d, +h, +mi);
        if (guess.getTime() < now.getTime() - 24 * 60 * 60 * 1000) year += 1;
      }
      return {
        kickoffAt: toLocalInput(year, month, +d, +h, +mi),
        rest: line.slice(0, m.index) + line.slice(m.index + m[0].length),
      };
    }
  }

  return { kickoffAt: '', rest: line };
}

function tidyLabel(s: string): string {
  const trimmed = s.replace(/\s+/g, ' ').trim().replace(/^[-,|–:]+|[-,|–:]+$/g, '').trim();
  if (!trimmed) return trimmed;
  const isShouting = trimmed === trimmed.toUpperCase() && /[A-Z]/.test(trimmed);
  const isFlat = trimmed === trimmed.toLowerCase() && /[a-z]/.test(trimmed);
  if (!isShouting && !isFlat) return trimmed;
  return trimmed
    // A token with a digit in it ("1X2", "2.5") is a code or a number, not a
    // word — title-casing it would turn "1X2" into "1x2".
    .replace(/\w\S*/g, (w) => (/\d/.test(w) ? w : w[0].toUpperCase() + w.slice(1).toLowerCase()))
    .replace(/\bVs\b/g, 'vs');
}

/** Splits on whichever delimiter appears, trying the least ambiguous first. */
function splitFields(text: string): string[] {
  for (const delim of ['|', '\t', '–', ',']) {
    if (text.includes(delim)) {
      return text.split(delim).map((p) => p.trim()).filter(Boolean);
    }
  }
  // " - " (hyphen with spaces on both sides) is common but risky: a fixture
  // written "Man United vs Man City" has no bare hyphen, so this is safe as
  // a fallback rather than a first choice.
  if (/\s-\s/.test(text)) {
    return text.split(/\s-\s/).map((p) => p.trim()).filter(Boolean);
  }
  return [text.trim()].filter(Boolean);
}

function splitFixtureFromRest(text: string): [string, string] {
  const m = /^(.*?\b(?:vs?|v)\b.*?)(?:[-:]|\s{2,})(.+)$/i.exec(text);
  if (m) return [m[1].trim(), m[2].trim()];
  return [text.trim(), ''];
}

export function parseTipLine(rawLine: string): ParsedTip {
  const line = rawLine.trim();
  if (!line) return { fixtureLabel: '', marketLabel: '', selectionLabel: '', odds: '', kickoffAt: '' };

  const { odds, rest: afterOdds } = findOdds(line);
  const { kickoffAt, rest: remainder } = findKickoff(afterOdds);

  const parts = splitFields(remainder);
  let fixtureLabel = '';
  let marketLabel = '';
  let selectionLabel = '';

  if (parts.length >= 3) {
    [fixtureLabel, marketLabel] = parts;
    selectionLabel = parts.slice(2).join(' ');
  } else if (parts.length === 2) {
    [fixtureLabel, selectionLabel] = parts;
    marketLabel = 'Prediction';
  } else if (parts.length === 1) {
    const [fixture, rest] = splitFixtureFromRest(parts[0]);
    fixtureLabel = fixture;
    if (rest) {
      marketLabel = 'Prediction';
      selectionLabel = rest;
    }
  }

  return {
    fixtureLabel: tidyLabel(fixtureLabel),
    marketLabel: tidyLabel(marketLabel),
    selectionLabel: tidyLabel(selectionLabel),
    odds,
    kickoffAt,
  };
}

export function parseTipsBlock(text: string): ParsedTip[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map(parseTipLine);
}
