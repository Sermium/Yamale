// What the custodian is trusting, said once and quietly.
//
// This used to be a screen. It should not be. For a rehearsal the coordinator is
// also every custodian, and a page that makes somebody acknowledge they might not
// trust themselves teaches them to click past warnings — which is how the real
// warning gets clicked past later, on the day, with five institutions watching.
//
// So: one line with a disclosure, appearing once near the start, never blocking
// anything. The mechanism it describes does not soften. The seed is still
// generated in the custodian's browser and the coordinator still cannot see it,
// because the ceremony this rehearses is one where five institutions do not trust
// whoever hosts the page. A rehearsal with a weaker mechanism would drill a flow
// nobody is going to run.

import { disclosure, el, paragraph } from './dom.ts';

// SCRIPT_NAME is the file this module is bundled into, and the one the browser
// can hash for itself. Named rather than derived from the URL so the comparison
// below is against a specific entry in the coordinator's manifest — comparing a
// measured digest against "whatever the server called it" would let a server
// that renamed the file answer with the digest of something else.
const SCRIPT_NAME = 'ceremony.js';

export function trustNote(bundleHash: string): HTMLElement {
  const verdict = el('p', { class: 'small' }, ['checking the code this page is running…']);
  const measured = el('p', { class: 'mono break tiny' }, ['']);

  void compareLoadedScript().then((result) => {
    measured.textContent = result.measured ?? '';
    if (!result.measured) {
      verdict.textContent = 'This browser would not let the page hash its own code, so it cannot check itself here.';
      return;
    }
    if (result.served === null) {
      verdict.textContent = 'The coordinator did not say which digest it served, so there is nothing to compare against.';
      return;
    }
    // Said as a verdict, not left as two hex strings for a person to compare
    // character by character. Two sixty-four-character values side by side is a
    // check nobody performs; a sentence is one they read.
    verdict.textContent =
      result.measured === result.served
        ? `The script this browser loaded is exactly the one the coordinator says it served.`
        : 'WARNING: the script this browser loaded is NOT the one the coordinator says it served. Stop and say so.';
  });

  return disclosure('What you are trusting here', [
    paragraph(
      'The words are created on this device and are never sent anywhere. What leaves is a public address, a ' +
        'public key and a signature — nothing that can spend anything.',
    ),
    paragraph(
      'That said: this page is code, served to you by whoever is running the ceremony. If they served a ' +
        'different page, it could send the words. Comparing the digest below against the one published in the ' +
        'repository proves the code you are running is the code that was reviewed. It does not prove the ' +
        'reviewed code is honest — only that you are running the same thing everybody else can read.',
    ),
    el('p', { class: 'small muted' }, ['Bundle digest, to compare against the published value:']),
    el('p', { class: 'mono break tiny' }, [bundleHash]),
    verdict,
    measured,
    paragraph(
      'The stronger option is the air-gapped one: the same ceremony run from a binary on a machine with no ' +
        'network, five people in a room. This is the networked version of it, and it exists because five ' +
        'people in five cities cannot use the other one.',
    ),
  ]);
}

// compareLoadedScript hashes the script the browser actually executed and asks
// the coordinator what it served.
//
// Measuring rather than displaying is the point. A digest the server asserts
// proves nothing about the bytes that ran; a digest computed from those bytes at
// least catches a coordinator who altered the code and forgot to alter the
// manifest. It cannot catch one who altered both — which is why the value only
// means anything against a digest published somewhere the coordinator does not
// control.
async function compareLoadedScript(): Promise<{ measured: string | null; served: string | null }> {
  const measured = await hashOwnScript();
  let served: string | null = null;
  try {
    const response = await fetch('api/bundle', { cache: 'no-store', credentials: 'omit' });
    const manifest = (await response.json()) as { files?: Record<string, string> };
    served = manifest.files?.[SCRIPT_NAME] ?? null;
  } catch {
    served = null;
  }
  return { measured, served };
}

async function hashOwnScript(): Promise<string | null> {
  try {
    // import.meta.url names the file actually executing, which
    // document.currentScript does not for a module.
    const response = await fetch(new URL(import.meta.url), { cache: 'no-store', credentials: 'omit' });
    const bytes = await response.arrayBuffer();
    const digest = await crypto.subtle.digest('SHA-256', bytes);
    return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('');
  } catch {
    // Over plain HTTP crypto.subtle does not exist at all. Reported rather than
    // left blank, because a missing measurement is itself worth knowing about.
    return null;
  }
}
