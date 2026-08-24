import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { CATALOGUES, en } from './strings.ts';

/**
 * The same guarantee the shared catalogue has, for the explorer's own keys.
 *
 * A missing key does not fail at runtime: `t()` falls back to English and, past
 * that, to the key itself. So an untranslated string appears in English in the
 * middle of an otherwise French page — which reads to that person as a bug and
 * to us as nothing at all. Adding a key to `en` and forgetting the other four is
 * one edit and no error.
 */
test('every locale carries every key English has', () => {
  const reference = Object.keys(en).sort();

  for (const [locale, catalogue] of Object.entries(CATALOGUES)) {
    const missing = reference.filter((k) => !(k in catalogue));
    assert.deepEqual(
      missing,
      [],
      `${locale} is missing ${missing.length} key(s) — they would render in English`,
    );
  }
});

test('no locale carries a key English has dropped', () => {
  for (const [locale, catalogue] of Object.entries(CATALOGUES)) {
    const dead = Object.keys(catalogue).filter((k) => !(k in en));
    assert.deepEqual(dead, [], `${locale} has ${dead.length} key(s) nothing reads any more`);
  }
});

test('nothing was left in English by copy-paste', () => {
  // The failure this catches is a translator's placeholder: a locale that
  // "has" the key because the English was pasted in and never replaced.
  // Proper nouns and codes are excluded, since NGN is NGN in every language.
  const identical: string[] = [];
  const allowed = new Set([
    'xp.status.seconds',
    'xp.status.ofSupply',
    'xp.feed.request',
    'xp.feed.inBlock',
  ]);

  for (const [locale, catalogue] of Object.entries(CATALOGUES)) {
    if (locale === 'en') continue;
    for (const [key, value] of Object.entries(catalogue)) {
      if (allowed.has(key)) continue;
      if (value === en[key as keyof typeof en]) identical.push(`${locale}:${key}`);
    }
  }

  assert.deepEqual(identical, [], 'these are still the English string');
});

test('every placeholder survives translation', () => {
  // `t()` interpolates {named} holes. A translation that drops one renders a
  // sentence with a missing number; one that renames it renders the literal
  // "{height}" to a reader.
  const holes = (s: string) => (s.match(/\{[a-zA-Z]+\}/g) ?? []).sort();

  for (const [locale, catalogue] of Object.entries(CATALOGUES)) {
    for (const [key, value] of Object.entries(catalogue)) {
      assert.deepEqual(
        holes(value),
        holes(en[key as keyof typeof en]),
        `${locale}:${key} does not carry the same placeholders as English`,
      );
    }
  }
});
