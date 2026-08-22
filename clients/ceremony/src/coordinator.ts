// The coordinator's screen.
//
// Everything the person running the ceremony does is here. The only thing they
// type at a terminal is the command that starts the server: no flags for the
// roster, no file paths for the record, no shell to read invite links out of.
//
// It is four things in order, because that is the order the hour happens in:
// set the ceremony up, hand out the links, watch, then read the group out. The
// watching screen is the one they sit on for most of it, so it is a board that
// names who has not done what rather than a progress bar — a relay that
// withheld or delayed one submission is only visible if the interface says which
// one is missing.

import { api, type Credential } from './api.ts';
import {
  button,
  clear,
  el,
  errorBox,
  field,
  muted,
  panel,
  paragraph,
  row,
  showFingerprint,
  statusPill,
  textInput,
} from './dom.ts';
import { qrSVG } from './qr.ts';
import type { CeremonyParams, SignedAttestation, Submission } from './wire.ts';
import { assembleGroup, type Assembled } from './group.ts';

type CustodianStatus = {
  name: string;
  phase: 'invited' | 'opened' | 'generated' | 'submitted' | 'attested';
  link?: string;
  issue: number;
  address?: string;
  fingerprint?: string;
  proved: boolean;
  waiting_since?: string;
};

type HostState = {
  ceremony: string;
  started_at: string;
  params_set: boolean;
  params: CeremonyParams;
  params_fingerprint: string;
  custodians: CustodianStatus[];
  missing: string;
  ready: boolean;
  submissions: Submission[];
  attestations: SignedAttestation[];
  bundle_hash: string;
  complete: boolean;
  notes: string[];
};

export async function runCoordinator(credential: Credential, root: HTMLElement): Promise<void> {
  // The coordinator's screen gets the desktop's width; the custodian's does not.
  // A custodian is reading one panel of prose on a phone, and the same measure
  // that suits five invite cards side by side would be the wrong shape for it.
  document.documentElement.classList.add('wide');

  let state = (await api.coordinatorState(credential)) as HostState;
  let message = '';
  let record: string | null = null;

  const render = () => {
    clear(root);
    root.append(header(state));
    if (message) root.append(errorBox(message));
    if (!state.params_set) {
      root.append(setupPanel(async (body) => {
        try {
          state = (await api.setup(credential, body)) as HostState;
          message = '';
        } catch (error) {
          message = error instanceof Error ? error.message : String(error);
        }
        render();
      }));
      root.append(bundlePanel(state));
      return;
    }
    root.append(agreedPanel(state));
    root.append(linksPanel(state, async (name, reason) => {
      try {
        state = (await api.reissue(credential, name, reason)) as HostState;
        message = '';
      } catch (error) {
        message = error instanceof Error ? error.message : String(error);
      }
      render();
    }));
    root.append(boardPanel(state));
    if (state.ready) root.append(groupPanel(state));
    root.append(
      recordPanel(state, record, async (body) => {
        try {
          const result = await api.exportRecord(credential, body);
          record = result.record;
          message = '';
        } catch (error) {
          message = error instanceof Error ? error.message : String(error);
        }
        render();
      }),
    );
    root.append(bundlePanel(state));
  };

  render();

  // Four seconds. Fast enough that the board feels live while five people work
  // through their own screens, slow enough that an hour of it is not a load
  // problem, and a poll that fails leaves the last known board on screen rather
  // than blanking it.
  window.setInterval(async () => {
    try {
      const next = (await api.coordinatorState(credential)) as HostState;
      if (JSON.stringify(next) !== JSON.stringify(state)) {
        state = next;
        render();
      }
    } catch {
      // Deliberately silent: the coordinator is watching a board, and a network
      // blip is not something to replace it with.
    }
  }, 4000);
}

function header(state: HostState): HTMLElement {
  return el('header', {}, [
    el('h1', {}, ['Key ceremony — coordinator']),
    state.params_set ? el('span', { class: 'pill mono' }, [state.params.ceremony_id]) : el('span', { class: 'pill' }, ['not set up']),
  ]);
}

function setupPanel(submit: (body: unknown) => Promise<void>): HTMLElement {
  const name = textInput('Yamale foundation key ceremony');
  const chain = textInput('yamale-1');
  const threshold = el('input', { type: 'number', min: '2', max: '15' });
  threshold.value = '3';
  const days = el('input', { type: 'number', min: '1', max: '90' });
  days.value = '7';
  const seq = el('input', { type: 'number', min: '0' });
  seq.value = '1';

  // Blank is the foundation. There is no toggle and no second screen, because the
  // country IS the difference: a ceremony with a perimeter is a country office's
  // and one without is the foundation's, and a checkbox that could disagree with
  // the field next to it would be a third state nobody wants.
  const country = textInput('', 'blank for the foundation, e.g. SN');
  const roles = textInput('', 'ROLE_PAYMENTS_AUTHORITY, ROLE_ENFORCEMENT_AUTHORITY');

  // Said as the coordinator types, because the consequence is not obvious from an
  // empty field: what changes is what the office is named on chain forever, and
  // what the super users are shown before they generate.
  const whose = muted('');
  const describe = () => {
    const code = country.value.trim().toUpperCase();
    whose.textContent = code === ''
      ? 'No country: this is the foundation ceremony. The group will be recorded as "Yamale foundation" and the ' +
        'ceremony writes the constitutional invariants for genesis.'
      : `A country office for ${code}. The group will be recorded as "${name.value.trim()} (${code})", every ` +
        'super user sees the country and the roles before they generate, and no genesis fragment is written — ' +
        "the office's group is created by a transaction on a running chain.";
  };
  describe();
  country.addEventListener('input', describe);
  name.addEventListener('input', describe);

  const roster = el('div', { class: 'roster' });
  const addRow = (value = '') => {
    const input = textInput(value, 'name, as it will appear in the record');
    roster.append(el('div', { class: 'roster__row' }, [input]));
  };
  for (let i = 0; i < 5; i++) addRow();

  return panel('Set the ceremony up', [
    paragraph(
      'These are the values every custodian will see a fingerprint of before they generate anything. Get them ' +
        'right now: they cannot change once somebody has submitted, because every fingerprint already read aloud ' +
        'covers them.',
    ),
    field('What this ceremony is called', name),
    field('Chain id', chain),
    el('div', { class: 'grid2' }, [
      field('Signatures required', threshold),
      field('Days to vote', days),
      field('Group policy sequence number', seq),
    ]),
    el('h3', {}, ['Whose keys these are']),
    field('Country (ISO 3166-1 alpha-2)', country),
    field('Roles this office will hold', roles),
    muted(
      'Both values go into the fingerprint every custodian reads aloud before generating. That is the whole ' +
        'point of them being here: without it, keys generated for one country could be used for an office ' +
        'granted authority over another, and nothing anybody saw would have said so.',
    ),
    whose,
    el('h3', {}, ['The custodians']),
    muted('One name per custodian. Each gets their own link, and a link speaks for that name only.'),
    roster,
    row(
      button('Add another custodian', () => addRow(), 'secondary'),
      button('Create the ceremony and the links', async () => {
        const names = Array.from(roster.querySelectorAll('input'))
          .map((input) => (input as HTMLInputElement).value.trim())
          .filter((value) => value !== '');
        await submit({
          ceremony: name.value.trim(),
          chain_id: chain.value.trim(),
          threshold: Number(threshold.value),
          custodians: names,
          policy_seq: Number(seq.value),
          // Sent as a Go duration string because that is what the parameters
          // carry and what the fingerprint covers. Days in the interface,
          // because "how many hours should a vote stay open" is not a question
          // anybody answers in hours.
          voting_period: `${Number(days.value) * 24}h0m0s`,
          // Uppercased and trimmed here as well as on the server, so what the
          // coordinator sees described above is what gets sent. The server
          // refuses anything its own normalisation would have had to change
          // beyond that, rather than rewriting a value nobody agreed to.
          country: country.value.trim().toUpperCase(),
          roles: roles.value
            .split(',')
            .map((role) => role.trim().toUpperCase())
            .filter((role) => role !== ''),
        });
      }),
    ),
  ]);
}

function agreedPanel(state: HostState): HTMLElement {
  const office = state.params.office;
  const facts: Array<Node | string> = [
    el('dt', {}, ['Ceremony']),
    el('dd', {}, [state.params.ceremony]),
    el('dt', {}, ['Chain']),
    el('dd', {}, [state.params.chain_id]),
    el('dt', {}, ['Threshold']),
    el('dd', {}, [`${state.params.threshold} of ${state.params.custodians.length}`]),
    el('dt', {}, ['Voting window']),
    el('dd', {}, [state.params.voting_period]),
  ];
  if (office) {
    facts.push(
      el('dt', {}, ['Country']),
      el('dd', { class: 'mono' }, [office.country]),
      el('dt', {}, ['Roles']),
      el('dd', { class: 'mono' }, [office.roles.join(', ')]),
      el('dt', {}, ['Recorded as']),
      el('dd', {}, [`${state.params.ceremony} (${office.country})`]),
    );
  } else {
    facts.push(el('dt', {}, ['Recorded as']), el('dd', {}, ['Yamale foundation']));
  }
  return panel('What everybody is agreeing to', [
    el('dl', { class: 'facts' }, facts),
    paragraph('Read this aloud before anybody generates a key. Every custodian\'s page shows the same value.'),
    showFingerprint(state.params_fingerprint),
  ]);
}

function linksPanel(state: HostState, reissue: (name: string, reason: string) => Promise<void>): HTMLElement {
  const cards = el('div', { class: 'cards' });
  for (const custodian of state.custodians) {
    if (!custodian.link) continue;
    const link = custodian.link;
    const copied = el('span', { class: 'small muted' }, ['']);
    cards.append(
      el('div', { class: 'card' }, [
        el('div', { class: 'card__name' }, [
          custodian.name,
          custodian.issue > 1 ? statusPill(`link ${custodian.issue}`, 'warn') : el('span', {}, []),
        ]),
        qrSVG(link),
        el('p', { class: 'mono break tiny' }, [link]),
        row(
          button('Copy link', () => {
            void navigator.clipboard.writeText(link).then(
              () => {
                copied.textContent = 'copied';
              },
              () => {
                copied.textContent = 'this browser refused the clipboard — select the link above';
              },
            );
          }, 'secondary'),
          copied,
        ),
        reissueControl(custodian, reissue),
      ]),
    );
  }

  return panel('One link per custodian', [
    paragraph(
      'Send each person their own link, by whatever channel you already use to reach them. A link identifies the ' +
        'custodian it was made for, so it cannot be redeemed by anybody else or used for a second name.',
    ),
    muted('The QR code is there because nobody types a link with a token in it correctly. Custodians on phones should scan.'),
    cards,
  ]);
}

// reissueControl is the answer to "I closed the tab".
//
// It carries its own cost, in the sentence next to the button, because the cost
// is different before and after the phrase was shown and the coordinator is the
// one who has to say so out loud on the call.
function reissueControl(
  custodian: CustodianStatus,
  reissue: (name: string, reason: string) => Promise<void>,
): HTMLElement {
  if (custodian.phase === 'submitted' || custodian.phase === 'attested') {
    return muted('This custodian has submitted. Their key is in the group; it cannot be reissued from here.');
  }
  const cost =
    custodian.phase === 'generated'
      ? 'They have already been shown twenty-four words. Reissuing abandons that key — they must destroy the ' +
        'sheet and start again. Nothing here ever held their words, so there is no way to show them again.'
      : 'Nothing has been generated on this link yet, so reissuing costs nothing but a new message.';
  const reason = textInput('', 'why (goes in the record)');
  return el('div', { class: 'reissue' }, [
    muted(cost),
    reason,
    row(
      button('Reissue this link', () => void reissue(custodian.name, reason.value.trim()), 'danger'),
    ),
  ]);
}

const PHASE_LABEL: Record<CustodianStatus['phase'], [string, string]> = {
  invited: ['invited — not opened', 'wait'],
  opened: ['opened the link', 'wait'],
  generated: ['has their words', 'wait'],
  submitted: ['sent their key', 'ok'],
  attested: ['attested', 'ok'],
};

function boardPanel(state: HostState): HTMLElement {
  const list = el('ul', { class: 'board' });
  for (const custodian of state.custodians) {
    const [label, kind] = PHASE_LABEL[custodian.phase];
    list.append(
      el('li', {}, [
        el('span', { class: 'board__name' }, [custodian.name]),
        statusPill(label, kind),
        el('span', { class: 'mono small' }, [custodian.fingerprint ?? '']),
        // Said, not implied. The first three phases are the page's own word for
        // it; the last two are backed by a signature this process verified. A
        // board that showed them identically would be claiming knowledge it
        // does not have.
        el('span', { class: 'small muted' }, [custodian.proved ? 'verified by signature' : 'reported by their page']),
      ]),
    );
  }

  const outstanding = state.custodians.filter((c) => c.phase !== 'attested').map((c) => c.name);
  return panel(state.complete ? 'Finished' : 'Where everybody is', [
    el('p', { class: 'waiting' }, [
      // Counted rather than written out as "all five": a country office is a
      // 2-of-3 or a 3-of-4 as often as it is a 3-of-5, and an interface that
      // hard-codes the foundation's shape is an interface that is wrong on the
      // screen somebody is checking.
      outstanding.length === 0
        ? `All ${state.custodians.length} have attested. Nothing is outstanding.`
        : `Waiting on: ${outstanding.join(', ')}.`,
    ]),
    list,
    muted(state.missing),
  ]);
}

function groupPanel(state: HostState): HTMLElement {
  let assembled: Assembled;
  try {
    // Computed here too, from the same submissions, so the coordinator can read
    // the value out. It is not authoritative and the custodians' pages never
    // display it: each of them computes their own, and the comparison is five
    // people saying sixteen characters to each other.
    assembled = assembleGroup(state.params, state.submissions);
  } catch (error) {
    return panel('The group does not add up', [errorBox(error instanceof Error ? error.message : String(error))]);
  }

  const members = el('ul', { class: 'plain' });
  for (const custodian of assembled.custodians) {
    const attested = state.attestations.some((a) => a.attestation.address === custodian.address);
    members.append(
      el('li', {}, [
        el('span', { class: 'mono' }, [custodian.fingerprint]),
        el('span', {}, [custodian.name]),
        attested ? statusPill('attested', 'ok') : statusPill('not yet', 'wait'),
      ]),
    );
  }

  const office = state.params.office;
  const children: Array<Node | string> = [
    paragraph('Read this out loud. Every custodian must be showing the same sixteen characters on their own device.'),
    showFingerprint(assembled.fingerprint),
    el('h3', {}, ['Policy address']),
    // Two different claims, because the address means two different things. For
    // the foundation it is a fact fixed by the same genesis file that fixes the
    // membership. For an office it is a guess about how many group policies the
    // running chain has created, and presenting a guess as an address is how
    // somebody ends up sending money to it.
    paragraph(
      office
        ? `Predicted from policy sequence ${state.params.policy_seq}. An office's group is created by a ` +
          'transaction on a running chain, so the chain decides the sequence and this is not the address until ' +
          '`ceremony country confirm` has read it back.'
        : `Where every seized asset on ${state.params.chain_id} is sent. It goes into genesis in two places, and the chain refuses to start if they disagree.`,
    ),
    copyable(assembled.policy_address),
    members,
  ];

  if (office) {
    children.push(
      el('h3', {}, ['What happens next']),
      paragraph(
        'Nothing here goes into a genesis file. Rendering the record below writes the group and the ' +
          "submissions it was computed from beside the server; `ceremony country` reads that to create the " +
          "office's group on the running chain and to verify its role grants afterwards.",
      ),
      el('ul', { class: 'plain small' }, [
        el('li', {}, [`${assembled.custodians.length} super users, threshold ${state.params.threshold}`]),
        el('li', {}, [`${office.country}: ${office.roles.join(', ')}`]),
        el('li', {}, [`group fingerprint ${assembled.fingerprint}`]),
      ]),
    );
    return panel('The group', children);
  }

  return panel('The group', [
    ...children,
    // No copy buttons for the genesis fragment and the invariants.
    //
    // They used to be here with "splice this into app_state.group", which was
    // worse than saying nothing: rendering the record writes both to disk beside
    // the server, and the genesis build reads that directory itself. Offering
    // them to be copied invited somebody to paste a fragment into a file by hand
    // — work that was already done, in a form where a truncated clipboard or a
    // stray character would produce a genesis that starts and disagrees.
    //
    // The values are still shown, because a coordinator should be able to see
    // what the ceremony produced. They are shown as a summary of what was
    // written, not as something to carry anywhere.
    el('h3', {}, ['What goes into genesis']),
    paragraph(
      'Nothing to copy. Rendering the record below writes the group and the constitutional ' +
        'invariants beside the server, and the genesis build reads that directory — pass it as ' +
        'CEREMONY_DIR and it splices both in itself.',
    ),
    el('ul', { class: 'plain small' }, [
      el('li', {}, [`${assembled.custodians.length} custodians, threshold ${state.params.threshold}`]),
      el('li', {}, [`recovery destination ${assembled.policy_address}`]),
      el('li', {}, [`group fingerprint ${assembled.fingerprint}`]),
    ]),
  ]);
}

function copyable(text: string): HTMLElement {
  const state = el('span', { class: 'small muted' }, ['']);
  return el('div', { class: 'copyable' }, [
    el('pre', { class: 'mono' }, [text]),
    row(
      button('Copy', () => {
        void navigator.clipboard.writeText(text).then(
          () => {
            state.textContent = 'copied';
          },
          () => {
            state.textContent = 'this browser refused the clipboard — select the text above';
          },
        );
      }, 'secondary'),
      state,
    ),
  ]);
}

function recordPanel(
  state: HostState,
  record: string | null,
  save: (body: unknown) => Promise<void>,
): HTMLElement {
  const location = textInput('', 'where this was run from');
  const notes = el('textarea', { placeholder: 'anything that happened: an abandoned key, an interruption, a link reissued' });

  const children: Array<Node | string> = [
    paragraph(
      'This is the last thing to do, and the only thing. The record is what somebody reads in five years to ' +
        'decide whether these five keys can be trusted — so it is rendered here to print and sign, and the ' +
        'same action writes the group and the constitutional invariants beside the server for the genesis ' +
        'build to read. Nothing needs copying anywhere by hand.',
    ),
    field('Location', location),
    field('Notes', notes),
    row(
      button('Render and export the record', () =>
        void save({
          location: location.value.trim(),
          participants: [],
          notes: notes.value
            .split('\n')
            .map((line) => line.trim())
            .filter((line) => line !== ''),
        }),
      ),
      button('Print this page', () => window.print(), 'secondary'),
    ),
  ];
  if (record) children.push(el('pre', { class: 'record' }, [record]));
  if (state.notes.length > 0) {
    children.push(el('h3', {}, ['Already noted']));
    children.push(el('ul', { class: 'plain' }, state.notes.map((note) => el('li', {}, [note]))));
  }
  return panel('The record', children);
}

function bundlePanel(state: HostState): HTMLElement {
  return panel('What the custodians are running', [
    muted(
      'The SHA-256 of the page served to every custodian. Publish it alongside the ceremony so anybody can check ' +
        'that the code generating the keys is the code in the repository.',
    ),
    el('p', { class: 'mono break small' }, [state.bundle_hash]),
  ]);
}
