// One custodian's journey, from opening a link to signing an attestation.
//
// It is one continuous flow with no step where the custodian leaves the page to
// do something by hand: no file to download, none to upload, no JSON to copy
// between devices. Everything public that has to move between the coordinator and
// a custodian moves over the connection they already have, which is the entire
// point of hosting it.
//
// The order of the steps is not arbitrary and two of them are load-bearing:
//
//   The words are removed from the document before the check is asked. Not
//   hidden with CSS, not scrolled off — removed, and the local variable holding
//   them cleared. The four words the custodian reads back after that are read
//   from paper or they are not read at all, and a check performed against a
//   screen that still has the answer on it is theatre.
//
//   The presence check comes before the attestation and refuses it. A custodian
//   in a room could see the five fingerprints on one screen; alone on a phone
//   they cannot, so their own address being absent from the computed group has
//   to be a refusal rather than a warning.

import { api, type Credential } from './api.ts';
import {
  button,
  clear,
  disclosure,
  el,
  errorBox,
  field,
  mono,
  muted,
  panel,
  paragraph,
  row,
  showFingerprint,
  statusPill,
  textInput,
} from './dom.ts';
import { assembleGroup, missingFrom, presence, type Assembled } from './group.ts';
import { checkPhrase, deriveKey, identityOf, newPhrase, normalisePhrase, sign, signSubmission, zero, type KeyPair } from './key.ts';
import { attestationCanonical, toBase64, type Attestation, type CeremonyParams, type Submission } from './wire.ts';
import { trustNote } from './trust.ts';

type InviteView = {
  name: string;
  phase: string;
  issue: number;
  generated: boolean;
  params_set: boolean;
  params: CeremonyParams;
  params_fingerprint: string;
  submissions: Submission[];
  waiting: string[];
  attested: string[];
  own_address: string;
  bundle_hash: string;
  threshold: number;
  complete: boolean;
};

type Step =
  | 'welcome'
  | 'generate'
  | 'words'
  | 'readback'
  | 'identity'
  | 'submitted'
  | 'group'
  | 'done'
  | 'reenter';

// PHRASE lives in this variable and nowhere else, for the length of one screen.
//
// Cleared the moment the custodian says the words are written down, before the
// read-back is asked. A page that kept them would be able to fill in the check
// that exists to prove the paper is right, and the custodian would never know
// their sheet was wrong.
let phrase: string | null = null;
let key: KeyPair | null = null;

// expected is the four words the read-back is checked against, and it is all
// that survives the words screen.
//
// Retaining four of twenty-four is not a weakening, and it is worth saying why
// rather than leaving it to look like one: this page is holding the private key
// itself until the attestation is signed, so four words add nothing an attacker
// with access to this page did not already have. What dropping the other twenty
// buys is the property that matters — there is no state anywhere from which the
// phrase can be shown again, on this device or on the coordinator's.
let expected: Map<number, string> = new Map();

export async function runCustodian(credential: Credential, root: HTMLElement): Promise<void> {
  let view = (await api.invite(credential)) as InviteView;
  await api.opened(credential).catch(() => undefined);

  let step: Step = initialStep(view);
  let message = '';

  const refresh = async () => {
    view = (await api.invite(credential)) as InviteView;
  };

  const render = () => {
    clear(root);
    root.append(header(view));
    if (message) root.append(errorBox(message));
    root.append(screen());
  };

  const fail = (error: unknown) => {
    message = error instanceof Error ? error.message : String(error);
    render();
  };

  const go = (next: Step) => {
    message = '';
    step = next;
    render();
  };

  function screen(): HTMLElement {
    switch (step) {
      case 'welcome':
        return welcomeScreen(view, () => go('generate'));
      case 'generate':
        return generateScreen(view, async () => {
          try {
            // The grant is spent on the coordinator BEFORE the words exist, so
            // a page that crashed between the two has cost a link rather than
            // produced a second phrase for the same custodian.
            await api.generated(credential);
            phrase = newPhrase();
            key = deriveKey(phrase, 0);
            go('words');
          } catch (error) {
            fail(error);
          }
        });
      case 'words':
        return wordsScreen(view, () => {
          const words = normalisePhrase(phrase ?? '').split(' ');
          expected = new Map(pickPositions(words.length).map((position) => [position, words[position - 1] as string]));
          // Cleared here, before the check is asked, not after it is passed.
          phrase = null;
          go('readback');
        });
      case 'readback':
        return readbackScreen(expected, () => go('identity'), fail);
      case 'identity':
        return identityScreen(view, key, async () => {
          try {
            if (!key) throw new Error('this page no longer holds the key');
            const id = identityOf(view.name, key, new Date());
            await api.submit(credential, signSubmission(view.params.ceremony_id, id, key.priv));
            await refresh();
            go('submitted');
          } catch (error) {
            fail(error);
          }
        });
      case 'submitted':
        return waitingScreen(view, async () => {
          try {
            await refresh();
            if (view.waiting.length === 0) go('group');
            else render();
          } catch (error) {
            fail(error);
          }
        });
      case 'group':
        return groupScreen(view, key, credential, async () => {
          await refresh();
          go('done');
        }, fail);
      case 'done':
        return doneScreen(view);
      case 'reenter':
        return reenterScreen(view, (entered) => {
          try {
            const derived = deriveKey(entered, 0);
            if (view.own_address !== '' && derived.address !== view.own_address) {
              throw new Error(
                'those words do not produce the key recorded for you in this ceremony. The sheet and the record ' +
                  'disagree, which means the sheet is wrong: tell the coordinator, destroy that sheet, and ' +
                  'generate again on a new invitation',
              );
            }
            if (key) zero(key.priv);
            key = derived;
            go(view.own_address === '' ? 'identity' : view.waiting.length === 0 ? 'group' : 'submitted');
          } catch (error) {
            fail(error);
          }
        }, fail);
    }
  }

  render();

  // Polled rather than pushed. A custodian waiting on four other people needs to
  // see movement, and the thing they must never see is an undifferentiated
  // spinner: waitingScreen names who has not submitted, because a relay
  // withholding one submission looks exactly like a slow connection otherwise.
  window.setInterval(async () => {
    if (step !== 'submitted' && step !== 'welcome') return;
    try {
      await refresh();
      render();
    } catch {
      // A failed poll is not worth interrupting the page for; the next one
      // either works or the custodian is looking at a stale board that says so.
    }
  }, 4000);
}

function initialStep(view: InviteView): Step {
  if (!view.params_set) return 'welcome';
  if (view.own_address !== '') return 'reenter';
  if (view.generated) return 'reenter';
  return 'welcome';
}

function header(view: InviteView): HTMLElement {
  return el('header', {}, [
    el('h1', {}, ['Key ceremony']),
    el('span', { class: 'pill' }, [view.name]),
    view.params_set ? el('span', { class: 'pill mono' }, [view.params.ceremony_id]) : el('span', {}, []),
  ]);
}

function welcomeScreen(view: InviteView, next: () => void): HTMLElement {
  if (!view.params_set) {
    return panel('Not started yet', [
      paragraph('The coordinator has not set this ceremony up. Leave this page open; it will follow along.'),
    ]);
  }
  const office = view.params.office;
  return panel(
    office
      ? `You are a super user for ${view.params.ceremony} (${office.country})`
      : `You are custodian for ${view.params.ceremony}`,
    [
    paragraph(
      'In a moment this page will create twenty-four words on this device. They are the only thing that ever ' +
        'recovers your key, they are shown once, and nobody else — including whoever is running this ' +
        'ceremony — ever sees them.',
    ),
    // Named here as well as on the generate screen. The welcome screen is the one
    // a super user reads before deciding to start at all, and "what am I holding
    // authority over" is the question they are answering by starting.
    office
      ? paragraph(
          `This key becomes one share of an office that will hold ${office.roles.join(' and ')} inside ` +
            `${office.country}. You will see both again, under the fingerprint, before you generate.`,
        )
      : el('span', {}, []),
    el('ol', { class: 'steps' }, [
      el('li', {}, ['Write the twenty-four words on paper. Not a photograph, not a note app.']),
      el('li', {}, ['Read four of them back, so a mis-copied word is caught now rather than in five years.']),
      el('li', {}, ['Send only the public half: an address and a signature proving you hold the key.']),
      el('li', {}, [`Wait for the other ${view.params.custodians.length - 1}, then compare one value out loud.`]),
      el('li', {}, ['Sign your attestation and put the sheet somewhere it will survive.']),
    ]),
    paragraph(
      'What you need before you start: a pen, a sheet of paper, and about fifteen minutes you will not be ' +
        'interrupted in.',
    ),
    muted('Open this link in a private window if you have not already — it keeps the URL out of your history.'),
    trustNote(view.bundle_hash),
    row(button('I have paper and a pen — begin', next)),
    ],
  );
}

// whatThisKeyIsFor is what the custodian is shown before the generate button.
//
// It exists because the office is inside the parameters fingerprint, and a value
// inside a fingerprint that nobody is shown is a value nobody agreed to. A super
// user generating a key for their country's payments authority is signing up to
// hold a share of an account that will be granted authority over one named
// perimeter; if that perimeter is not on this screen, a coordinator could take
// the keys generated "for Senegal" and stand up an office over Nigeria, and every
// check afterwards would agree because they all read the same field.
//
// Rendered as its own block rather than folded into the ceremony line, because
// the country and the roles are the two things they are being asked to remember
// long enough to say out loud on the call.
function whatThisKeyIsFor(view: InviteView): HTMLElement {
  const office = view.params.office;
  if (!office && view.params.foundation_administrators) {
    // Said in as many words, because it is the most consequential thing anybody
    // is asked to generate a key for on this chain and the parameter name gives
    // no hint of it. "foundation_administrators" reads like a list of people with
    // logins.
    return el('div', {}, [
      paragraph(`Ceremony: ${view.params.ceremony} · chain ${view.params.chain_id}`),
      paragraph(
        'This key becomes one share of a group intended to be appointed a FOUNDATION ADMINISTRATOR. ' +
          "An administrator may correct the country recorded against any account on this chain — which moves " +
          'that account out from under the authority investigating it, and retires and reissues its identifier ' +
          '— and may hold an identifier with no country at all.',
      ),
      muted(
        'The group holds none of that yet. It is a parameter of x/alias, and only an ordinary governance ' +
          "proposal can add to it; the foundation's own 3-of-5 cannot. This is not the foundation account, and " +
          'no seized assets are sent here.',
      ),
    ]);
  }
  if (!office) {
    return el('div', {}, [
      paragraph(`Ceremony: ${view.params.ceremony} · chain ${view.params.chain_id}`),
      muted('No country: this is the foundation ceremony, which belongs to no national perimeter.'),
    ]);
  }
  return el('div', {}, [
    paragraph(`Ceremony: ${view.params.ceremony} · chain ${view.params.chain_id}`),
    el('dl', { class: 'facts' }, [
      el('dt', {}, ['Country']),
      el('dd', { class: 'mono' }, [office.country]),
      el('dt', {}, ['Authority this office will hold']),
      el('dd', { class: 'mono' }, [office.roles.join(', ')]),
      el('dt', {}, ['Recorded on chain as']),
      el('dd', {}, [`${view.params.ceremony} (${office.country})`]),
    ]),
    paragraph(
      `Your key becomes one share of an office holding that authority inside ${office.country}, and nowhere ` +
        'else. Both values are covered by the fingerprint below: if either is wrong, the fingerprint is wrong ' +
        'too, and this is the moment to say so — not after you have written twenty-four words down.',
    ),
  ]);
}

function generateScreen(view: InviteView, generate: () => void): HTMLElement {
  return panel('Create your key', [
    paragraph(
      'This happens on this device. The words appear once and this page will not show them again — there is ' +
        'nowhere to fetch them from, so nobody can.',
    ),
    whatThisKeyIsFor(view),
    el('div', {}, [
      muted('The value below is what the coordinator and every other custodian should also be showing. If it differs, stop.'),
      showFingerprint(view.params_fingerprint),
    ]),
    row(button('Generate my twenty-four words', generate)),
  ]);
}

function wordsScreen(view: InviteView, written: () => void): HTMLElement {
  const words = normalisePhrase(phrase ?? '').split(' ');
  const grid = el('div', { class: 'phrase' });
  words.forEach((word, index) => {
    grid.append(
      el('div', { class: 'word' }, [
        el('span', { class: 'word__n' }, [String(index + 1)]),
        el('span', { class: 'word__w' }, [word]),
      ]),
    );
  });

  const confirm = el('input', { type: 'checkbox', id: 'written' });
  const proceed = button('The words are on paper — continue', () => {
    if (!confirm.checked) return;
    written();
  });
  proceed.disabled = true;
  confirm.addEventListener('change', () => {
    proceed.disabled = !confirm.checked;
  });

  return panel(`${view.name}: write these down`, [
    paragraph('In this order, numbered. When you leave this screen they are gone from this page for good.'),
    grid,
    el('div', { class: 'confirm' }, [confirm, el('label', { for: 'written' }, ['I have written all twenty-four words on paper, in order.'])]),
    row(proceed),
    muted('Not written them down? Stay on this screen. Nothing else can bring them back.'),
  ]);
}

// pickPositions chooses which words to ask for.
//
// Four positions, and the last word is always one of them. The last word carries
// the checksum, so it is both the one most worth verifying and the one a
// transcriber is most likely to rush; asking a random four without it would leave
// the most valuable word unchecked most of the time.
export function pickPositions(count: number): number[] {
  const chosen = new Set<number>([count]);
  const random = new Uint32Array(16);
  crypto.getRandomValues(random);
  let at = 0;
  while (chosen.size < 4 && at < random.length) {
    const candidate = ((random[at] as number) % count) + 1;
    at += 1;
    chosen.add(candidate);
  }
  return [...chosen].sort((a, b) => a - b);
}

function readbackScreen(
  wanted: Map<number, string>,
  done: () => void,
  fail: (error: unknown) => void,
): HTMLElement {
  const positions = [...wanted.keys()];
  const inputs = new Map(positions.map((position) => [position, textInput()]));

  return panel('Read four words back from your sheet', [
    paragraph(
      'Read these off the paper, not off memory. A mis-copied word derives a different, empty account rather ' +
        'than failing, which is why it is checked now and not in five years.',
    ),
    ...positions.map((position) => field(`Word ${position}`, inputs.get(position) as HTMLInputElement)),
    row(
      button('Check my sheet', () => {
        const wrong: number[] = [];
        for (const position of positions) {
          const typed = (inputs.get(position) as HTMLInputElement).value.trim().toLowerCase();
          if (typed !== wanted.get(position)) wrong.push(position);
        }
        if (wrong.length > 0) {
          // Which words are wrong, not how many. A custodian told "one of these
          // is wrong" checks all four again and usually re-reads the same
          // mistake; told "word 17", they look at word 17.
          fail(
            new Error(
              `Word ${wrong.join(' and word ')} does not match what was generated. Check the sheet against ` +
                'nothing — the words are gone from this page. If you cannot make it match, the sheet is wrong: ' +
                'tell the coordinator, destroy it, and start again on a new invitation.',
            ),
          );
          return;
        }
        done();
      }),
    ),
  ]);
}

function identityScreen(view: InviteView, keypair: KeyPair | null, submit: () => void): HTMLElement {
  if (!keypair) return panel('Nothing to show', [paragraph('This page no longer holds your key.')]);
  return panel('Your address and fingerprint', [
    paragraph('Write the fingerprint on the same sheet as the words. It is what proves, years from now, that a sealed envelope holds the key it claims to.'),
    showFingerprint(keypair.fingerprint),
    el('p', { class: 'mono break' }, [keypair.address]),
    muted('Neither of these is secret. Both are in the record and in the file that launches the chain.'),
    paragraph(
      'Sending now transmits the public half only: this address, the public key, and a signature proving you ' +
        'hold the private half. The words stay on your paper.',
    ),
    row(button('Send my public half', submit)),
  ]);
}

function waitingScreen(view: InviteView, poll: () => void): HTMLElement {
  const list = el('ul', { class: 'plain' });
  for (const name of view.params.custodians) {
    const submitted = view.submissions.some((s) => s.identity.name === name);
    const attested = view.attested.includes(name);
    list.append(
      el('li', {}, [
        el('span', {}, [name]),
        attested
          ? statusPill('attested', 'ok')
          : submitted
            ? statusPill('sent their key', 'ok')
            : statusPill('not yet', 'wait'),
      ]),
    );
  }

  const ready = view.waiting.length === 0;
  return panel(ready ? 'Everybody has sent their key' : 'Waiting for the others', [
    paragraph(
      ready
        ? 'This page can now compute the group for itself. Nobody is computing it on your behalf.'
        : missingFromText(view),
    ),
    list,
    row(
      ready
        ? button('Compute the group on this device', poll)
        : button('Check again', poll, 'secondary'),
    ),
    muted('Your key is safe on your paper. This screen refreshes itself; you can leave it open.'),
  ]);
}

function missingFromText(view: InviteView): string {
  return missingFrom(view.params, view.submissions);
}

function groupScreen(
  view: InviteView,
  keypair: KeyPair | null,
  credential: Credential,
  done: () => Promise<void>,
  fail: (error: unknown) => void,
): HTMLElement {
  let assembled: Assembled;
  try {
    // Computed here, on this device, from the submissions as relayed. The
    // coordinator's own value is never displayed: a custodian who read the
    // fingerprint off the coordinator's screen would be trusting the one party
    // a 3-of-5 exists to distrust.
    assembled = assembleGroup(view.params, view.submissions);
  } catch (error) {
    return panel('This group does not add up', [
      errorBox(error instanceof Error ? error.message : String(error)),
      paragraph('Do not attest to anything. Say this out loud on the call.'),
    ]);
  }

  let absent: string | null = null;
  if (keypair) {
    try {
      presence(assembled, keypair.address, keypair.fingerprint);
    } catch (error) {
      absent = error instanceof Error ? error.message : String(error);
    }
  }

  const rows = el('ul', { class: 'plain' });
  for (const custodian of assembled.custodians) {
    const isOwn = keypair !== null && custodian.address === keypair.address;
    rows.append(
      el('li', { class: isOwn ? 'own' : '' }, [
        el('span', { class: 'mono' }, [custodian.fingerprint]),
        el('span', {}, [custodian.name]),
        isOwn ? statusPill('this is you', 'ok') : el('span', {}, []),
      ]),
    );
  }

  const sealed = el('input', { type: 'checkbox', id: 'sealed' });
  const drilled = el('input', { type: 'checkbox', id: 'drilled' });

  const office = view.params.office;
  const children: Array<Node | string> = [
    paragraph(
      `Read this out loud on the call. All ${view.params.custodians.length} of you must be showing the same ` +
        'sixteen characters.',
    ),
    showFingerprint(assembled.fingerprint),
    // Two different claims, because the address means two different things. For
    // the foundation it is fixed by the same genesis file that fixes the
    // membership. For a country office it is derived from a policy sequence number
    // the running chain has not reached yet, so presenting it as the office's
    // address would be presenting a guess as a fact.
    office
      ? paragraph(
          `This office's group policy address, predicted from policy sequence ${view.params.policy_seq}. The ` +
            'chain decides the real one when the group is created, so do not send anything to it on the ' +
            'strength of this screen:',
        )
      : paragraph(`Foundation policy address — where every seized asset on ${view.params.chain_id} will be sent:`),
    el('p', { class: 'mono break' }, [assembled.policy_address]),
    office
      ? el('div', {}, [
          el('h3', {}, ['What this office will hold']),
          el('dl', { class: 'facts' }, [
            el('dt', {}, ['Country']),
            el('dd', { class: 'mono' }, [office.country]),
            el('dt', {}, ['Authority']),
            el('dd', { class: 'mono' }, [office.roles.join(', ')]),
          ]),
        ])
      : el('span', {}, []),
    el('h3', {}, [`The ${assembled.custodians.length} custodians in this group`]),
    rows,
  ];

  if (absent) {
    children.push(errorBox(absent));
    children.push(
      paragraph(
        'This page will not let you attest to a group your own key is not in. That is the point: nobody can ' +
          'quietly leave you out of the account you are supposed to hold a share of.',
      ),
    );
  } else {
    children.push(
      el('div', { class: 'confirm' }, [drilled, el('label', { for: 'drilled' }, ['I recovered my key from the sheet and it matched.'])]),
      el('div', { class: 'confirm' }, [sealed, el('label', { for: 'sealed' }, ['The sheet is in a sealed envelope.'])]),
      row(
        button('Sign my attestation', async () => {
          try {
            if (!keypair) throw new Error('this page no longer holds your key');
            presence(assembled, keypair.address, keypair.fingerprint);
            const attestation: Attestation = {
              ceremony_id: view.params.ceremony_id,
              name: view.name,
              address: keypair.address,
              group_fingerprint: assembled.fingerprint,
              policy_address: assembled.policy_address,
              transcription_verified: true,
              restore_drill_passed: drilled.checked,
              envelope_sealed: sealed.checked,
              // False, and said plainly rather than left out. This flag exists
              // so a reader years from now knows which keys were generated
              // somewhere a hypervisor operator could have snapshotted memory;
              // a browser on a custodian's own device is not that, and the
              // hosted path's own caveat is the served code, which the record
              // carries as the bundle hash instead.
              virtualised: false,
              signed_at: new Date().toISOString().slice(0, 19) + 'Z',
            };
            const signature = toBase64(sign(attestationCanonical(attestation), keypair.priv));
            await api.attest(credential, {
              attestation,
              pubkey: { '@type': '/cosmos.crypto.secp256k1.PubKey', key: toBase64(keypair.pub) },
              signature,
            });
            await done();
          } catch (error) {
            fail(error);
          }
        }),
      ),
    );
  }

  return panel('The group', children);
}

function doneScreen(view: InviteView): HTMLElement {
  return panel('Done', [
    paragraph('Your attestation is signed and recorded. There is nothing else for you to do on this device.'),
    el('ol', { class: 'steps' }, [
      el('li', {}, ['Seal the sheet in the envelope, sign across the seal, and store it where you said you would.']),
      el('li', {}, ['Close this tab. Nothing here is worth keeping and nothing was stored.']),
      el('li', {}, [`Any ${view.threshold} of ${view.params.custodians.length} custodians can act together. One of you, alone, can do nothing — including you.`]),
    ]),
    muted('If you ever need to check the envelope holds the right key, recover it and compare the fingerprint you wrote on the sheet.'),
  ]);
}

function reenterScreen(
  view: InviteView,
  resume: (phrase: string) => void,
  fail: (error: unknown) => void,
): HTMLElement {
  const input = el('textarea', { placeholder: 'the twenty-four words, in order, separated by spaces', autocomplete: 'off', spellcheck: 'false' });
  return panel('Enter your twenty-four words to carry on', [
    paragraph(
      view.own_address === ''
        ? 'A phrase was already generated on this link, and nothing on the coordinator\'s side ever held it. If ' +
            'you have the words on paper, type them in and the ceremony continues from here.'
        : 'This page holds nothing between visits, so it needs your words again to sign for you. This is also the ' +
            'second check on your sheet — the one that matters, because it is the sheet as it now stands.',
    ),
    input,
    row(
      button('Continue', () => {
        const value = input.value;
        if (!checkPhrase(value)) {
          fail(
            new Error(
              'that is not a valid recovery phrase — the checksum does not match. One of the words is wrong, or ' +
                'two are swapped. Check it against the sheet word by word. Do not correct a guess and try again: ' +
                'a phrase with one wrong word derives a different, empty account rather than failing',
            ),
          );
          return;
        }
        input.value = '';
        resume(value);
      }),
    ),
    muted(
      'No words? Ask the coordinator to reissue your invitation. That abandons the key generated here — destroy ' +
        'any sheet you started, because it will not be in the group.',
    ),
  ]);
}
