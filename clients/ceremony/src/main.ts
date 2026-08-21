// The entry point: one bundle, two flows, decided by the link.
//
// ?i= is a custodian's invitation and ?t= is the coordinator's. One build rather
// than two, because the coordinator publishes one digest and a custodian
// comparing it needs the file they loaded to be the file that was published.

import { el, errorBox, panel, paragraph } from './dom.ts';
import { runCoordinator } from './coordinator.ts';
import { runCustodian } from './custodian.ts';

const root = document.getElementById('app');
if (!root) throw new Error('the page has no root element');

const query = new URLSearchParams(location.search);
const invite = query.get('i');
const coordinator = query.get('t');

// Held in a variable, never stored. Nothing in this bundle writes to
// localStorage, sessionStorage, IndexedDB or a cookie — not a theme, not a
// language, not a progress marker — so a custodian who forgot to open a private
// window has left nothing behind but a URL in their history.
// src/storage.test.ts reads the built bundle and fails if any storage API
// appears in it.

async function start(): Promise<void> {
  try {
    if (invite) {
      await runCustodian({ kind: 'custodian', token: invite }, root as HTMLElement);
      return;
    }
    if (coordinator) {
      await runCoordinator({ kind: 'coordinator', token: coordinator }, root as HTMLElement);
      return;
    }
    (root as HTMLElement).append(
      panel('This page needs your invitation link', [
        paragraph(
          'A key ceremony is reached by a link issued to one named custodian. Open the one you were sent, in ' +
            'full — the token at the end is part of it.',
        ),
        paragraph('If you are running the ceremony, use the coordinator link the server printed when it started.'),
      ]),
    );
  } catch (error) {
    (root as HTMLElement).append(
      el('div', {}, [
        errorBox(error instanceof Error ? error.message : String(error)),
        paragraph('Nothing has been generated and nothing has been lost. Reload, or ask the coordinator.'),
      ]),
    );
  }
}

void start();
