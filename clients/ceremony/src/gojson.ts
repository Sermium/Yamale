// Go's JSON, reproduced.
//
// This module exists for one reason: the group fingerprint five custodians read
// aloud to each other is taken over the raw bytes of the genesis fragment, and
// those bytes come out of Go's encoding/json. So the browser cannot use
// JSON.stringify. The two agree on most input and disagree on exactly the input
// a real roster contains.
//
// Three differences, all of them silent:
//
//   - Go escapes <, > and & as backslash-u escapes by default, because its
//     encoder assumes the output might land in an HTML document.
//     JSON.stringify does not. A custodian from "Chipo & Sons" would give the
//     browser and the binary different fingerprints from the same five
//     submissions — and the read-aloud step cannot tell that apart from an
//     attack, which is the whole reason it exists.
//
//   - Go escapes U+2028 and U+2029. JSON.stringify leaves them raw. Same
//     failure, rarer input.
//
//   - Go emits a lone surrogate as U+FFFD; JSON.stringify emits it escaped.
//     Unreachable from a text input in practice, handled because "unreachable in
//     practice" is how the other two got missed.
//
// Everything else — the two-character escapes, control characters as \u00xx,
// non-ASCII passed through as UTF-8 — matches, and is written out below rather
// than assumed.

const TWO_CHAR: Record<number, string> = {
  0x08: '\\b',
  0x09: '\\t',
  0x0a: '\\n',
  0x0c: '\\f',
  0x0d: '\\r',
  0x22: '\\"',
  0x5c: '\\\\',
};

function u(code: number): string {
  return `\\u${code.toString(16).padStart(4, '0')}`;
}

// goJSONString quotes a string the way Go's encoding/json does.
export function goJSONString(value: string): string {
  let out = '"';
  for (const ch of value) {
    const code = ch.codePointAt(0) as number;
    const short = TWO_CHAR[code];
    if (short !== undefined) {
      out += short;
      continue;
    }
    if (code < 0x20) {
      out += u(code);
      continue;
    }
    if (code === 0x3c || code === 0x3e || code === 0x26) {
      out += u(code);
      continue;
    }
    if (code === 0x2028 || code === 0x2029) {
      out += u(code);
      continue;
    }
    if (code >= 0xd800 && code <= 0xdfff) {
      out += '�';
      continue;
    }
    out += ch;
  }
  return `${out}"`;
}

// goJSONObject writes an object with the fields in the order given.
//
// Order is an argument rather than a property of a JavaScript object, because the
// order here is protobuf field-number order and it is load-bearing: the digest is
// over these bytes, and a refactor that reordered two keys would move the value
// five custodians compare over a telephone.
export function goJSONObject(fields: Array<[string, string]>): string {
  return `{${fields.map(([key, raw]) => `${goJSONString(key)}:${raw}`).join(',')}}`;
}

export function goJSONArray(items: string[]): string {
  return `[${items.join(',')}]`;
}

// indentGoJSON reproduces json.MarshalIndent with a two-space indent.
//
// Used for the constitution fragment, which Go writes indented and which the
// group fingerprint also covers. Implemented by re-emitting rather than by
// parsing and re-serialising, so the escaping above is the only escaping in play.
export function indentGoJSON(fields: Array<[string, string]>): string {
  if (fields.length === 0) return '{}';
  const lines = fields.map(([key, raw]) => `  ${goJSONString(key)}: ${raw}`);
  return `{\n${lines.join(',\n')}\n}`;
}

// protoDuration renders a duration the way protobuf JSON does: seconds, then a
// fraction in groups of three digits, then "s".
//
// The grouping is not cosmetic. A voting period of one and a half seconds is
// "1.500s", not "1.5s", and this string goes inside the genesis fragment the
// fingerprint covers.
export function protoDuration(nanos: bigint): string {
  const negative = nanos < 0n;
  const magnitude = negative ? -nanos : nanos;
  const seconds = magnitude / 1000000000n;
  const fraction = magnitude % 1000000000n;
  let text = seconds.toString();
  if (fraction !== 0n) {
    let digits = fraction.toString().padStart(9, '0');
    // Three, six or nine digits: protobuf JSON trims to whichever group holds
    // all the significant digits and no further.
    if (digits.endsWith('000000')) digits = digits.slice(0, 3);
    else if (digits.endsWith('000')) digits = digits.slice(0, 6);
    text += `.${digits}`;
  }
  return `${negative ? '-' : ''}${text}s`;
}

const DURATION_UNITS: Record<string, bigint> = {
  ns: 1n,
  us: 1000n,
  µs: 1000n,
  μs: 1000n,
  ms: 1000000n,
  s: 1000000000n,
  m: 60000000000n,
  h: 3600000000000n,
};

// parseGoDuration reads the duration string carried in the ceremony parameters.
//
// The parameters are agreed as text and their fingerprint is read aloud before
// anybody generates a key, so the browser has to interpret "168h0m0s" the way
// Go's time.ParseDuration does — including the fractional forms, because a
// coordinator can type one and the params fingerprint will have covered it.
export function parseGoDuration(text: string): bigint {
  const trimmed = text.trim();
  if (trimmed === '0' || trimmed === '+0' || trimmed === '-0') return 0n;

  let rest = trimmed;
  let sign = 1n;
  if (rest.startsWith('-')) {
    sign = -1n;
    rest = rest.slice(1);
  } else if (rest.startsWith('+')) {
    rest = rest.slice(1);
  }
  if (rest === '') throw new Error(`"${text}" is not a duration`);

  let total = 0n;
  while (rest !== '') {
    const match = /^(\d+(?:\.\d*)?|\.\d+)([a-zµμ]+)/.exec(rest);
    if (!match) throw new Error(`"${text}" is not a duration`);
    const [whole, number, unit] = match as unknown as [string, string, string];
    const scale = DURATION_UNITS[unit];
    if (scale === undefined) throw new Error(`"${text}" uses an unknown unit "${unit}"`);

    const dot = number.indexOf('.');
    const integerPart = dot < 0 ? number : number.slice(0, dot);
    const fractionPart = dot < 0 ? '' : number.slice(dot + 1);
    total += BigInt(integerPart === '' ? '0' : integerPart) * scale;
    if (fractionPart !== '') {
      // Scaled with integer arithmetic rather than a float, so "0.1s" is
      // exactly 100000000ns rather than whatever a double rounds to.
      const padded = (fractionPart + '000000000').slice(0, 9);
      total += (BigInt(padded) * scale) / 1000000000n;
    }
    rest = rest.slice(whole.length);
  }
  return sign * total;
}
