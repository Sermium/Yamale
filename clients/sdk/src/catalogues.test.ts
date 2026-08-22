import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { AVAILABLE, ar, en, fr, pt, sw } from './catalogues.ts';

const BY_LOCALE: Record<string, Record<string, string>> = { en, fr, ar, pt, sw };

/**
 * CLDR's plural categories.
 *
 * A locale carries one entry per category its own grammar uses, so Arabic has
 * six where English has two — `plural()` selects between them with the
 * platform's rules. That means "this locale has keys English does not" is
 * correct and expected for plural families, and only suspicious anywhere else.
 * Encoding that distinction is the difference between a useful test and one
 * somebody deletes.
 */
const CATEGORIES = ['zero', 'one', 'two', 'few', 'many', 'other'];

/** The plural family a key belongs to, or null if it is an ordinary key. */
function pluralStem(key: string): string | null {
  const cut = key.lastIndexOf('.');
  if (cut < 0) return null;
  return CATEGORIES.includes(key.slice(cut + 1)) ? key.slice(0, cut) : null;
}

/**
 * The catalogues have to hold the same keys, and nothing was checking.
 *
 * A missing key does not fail: `t()` falls back to English, and failing that to
 * the key itself. So an untranslated string appears in English to somebody using
 * the wallet in French — which looks like a rendering bug to them and like
 * nothing at all to us. On a product sold to francophone, lusophone, arabophone
 * and swahili-speaking institutions, a silently-English sentence in the middle
 * of a payment flow is a defect, not a cosmetic gap.
 *
 * The failure mode is asymmetric, which is why it needs a test rather than
 * care: adding a key to `en` and forgetting the other four is one edit and no
 * error, while noticing it means reading the app in a language you may not
 * speak.
 */
test('every locale carries every key English has', () => {
  const reference = Object.keys(en).sort();

  for (const locale of AVAILABLE) {
    const catalogue = BY_LOCALE[locale];
    assert.ok(catalogue, `${locale} is advertised in AVAILABLE but has no catalogue`);

    const missing = reference.filter((k) => !(k in catalogue));
    assert.deepEqual(missing, [],
      `${locale} is missing ${missing.length} key(s) that en has — they will render in English`);
  }
});

test('a key no locale recognises is dead, unless it is a plural form', () => {
  for (const locale of AVAILABLE) {
    const catalogue = BY_LOCALE[locale]!;

    const unexplained = Object.keys(catalogue).filter((k) => {
      if (k in en) return false;
      const stem = pluralStem(k);
      // A plural variant is legitimate when the family exists in English. What
      // is not legitimate is a variant of a family nobody declared, which is
      // how a renamed key leaves a translated orphan behind.
      return !(stem && `${stem}.other` in en);
    }).sort();

    assert.deepEqual(unexplained, [],
      `${locale} has ${unexplained.length} key(s) that are neither in en nor a plural form of one`);
  }
});

test('no locale leaves a key empty', () => {
  // Worse than a missing key: an empty string defeats the fallback and renders
  // as nothing at all, so a label simply vanishes with no clue where it went.
  for (const locale of AVAILABLE) {
    for (const [key, value] of Object.entries(BY_LOCALE[locale]!)) {
      assert.ok(String(value).trim() !== '', `${locale}.${key} is empty`);
    }
  }
});

test('no translation invents a placeholder', () => {
  // `t()` substitutes {name}-style placeholders from the values it is given, so
  // a placeholder the caller never passes renders its own braces on screen.
  //
  // Asserted as a subset rather than as equality, because *dropping* one is
  // sometimes the correct translation: Arabic's zero form is "no transactions",
  // which needs no count in it. Requiring equality would force a wrong
  // translation to satisfy a test.
  const placeholders = (s: string) => new Set(s.match(/\{[a-zA-Z0-9_]+\}/g) ?? []);

  for (const locale of AVAILABLE) {
    if (locale === 'en') continue;
    for (const [key, value] of Object.entries(BY_LOCALE[locale]!)) {
      const stem = pluralStem(key);
      const reference = (en as Record<string, string>)[key]
        ?? (stem ? (en as Record<string, string>)[`${stem}.other`] : undefined);
      if (reference === undefined) continue;

      const allowed = placeholders(reference);
      for (const ph of placeholders(String(value))) {
        assert.ok(allowed.has(ph),
          `${locale}.${key} uses ${ph}, which en does not supply — it will render literally`);
      }
    }
  }
});

test('AVAILABLE lists exactly the catalogues that exist', () => {
  assert.deepEqual([...AVAILABLE].sort(), Object.keys(BY_LOCALE).sort());
});
