// The foundation console's judgement, separated from its rendering.
//
// Everything here is a pure function over what the chain returned, and it lives
// in its own module for one reason: these are the calculations a custodian is
// trusting when they vote. Whether a proposal has enough votes, what it would
// actually do, and whether a membership change is a shape the chain will accept
// are each a decision that has to be right, and none of them can be checked by
// looking at a rendered page. So they are here, and they have tests.
//
// The page itself does no arithmetic on votes and no interpretation of messages.

/**
 * How long after the voting period ends a passed proposal can still be executed.
 *
 * x/group publishes no params query, so this cannot be read from the chain — it
 * is the `MaxExecutionPeriod` compiled into the module config at
 * app/app_config.go, and if that changes this constant has to change with it.
 *
 * It is here rather than left out because it is a deadline that silently
 * destroys a decision. A proposal that passed and was never executed stops
 * being executable fourteen days after its voting period ends, and at that
 * point the custodians' decision is simply gone — no event, no status change
 * anyone is watching, just a proposal that can no longer be run. A console that
 * showed the voting deadline and not this one would be showing the deadline
 * that does not lose money.
 */
export const MAX_EXECUTION_PERIOD_SECONDS = 1209600;

/** Vote weight is per-member; the foundation gives every custodian one. */
const YES = 'VOTE_OPTION_YES';
const NO = 'VOTE_OPTION_NO';
const ABSTAIN = 'VOTE_OPTION_ABSTAIN';
const VETO = 'VOTE_OPTION_NO_WITH_VETO';

/**
 * A custodian's name and key fingerprint, read from their group member metadata.
 *
 * The ceremony writes `chris2 (XYEC-D45D)` into each member's metadata, and the
 * fingerprint is the whole point of showing it: it is what a custodian compares
 * against the paper in front of them to confirm the group on screen is the group
 * they attested to. An address alone cannot be checked against anything a person
 * carries.
 *
 * Unparseable metadata yields a null name rather than a guess. A custodian who
 * cannot see a fingerprint must be told there is none, because the alternative
 * is them reading a truncated address and believing they have checked something.
 */
export function custodianIdentity(metadata) {
  const raw = String(metadata ?? '').trim();
  const m = /^(.*?)\s*\(([^()]+)\)\s*$/.exec(raw);
  if (!m) return { name: raw || null, fingerprint: null };
  return { name: m[1].trim() || null, fingerprint: m[2].trim() || null };
}

/**
 * The members of the group as this console works with them.
 *
 * `weight` is kept as a number because every vote calculation below is weighted
 * — the foundation happens to give everyone 1, but a console that hardcoded
 * that assumption would miscount the first time it did not hold, and would do
 * so silently.
 */
export function toCustodians(members) {
  const custodians = (members ?? []).map((entry) => {
    const member = entry.member ?? entry;
    const identity = custodianIdentity(member.metadata);
    return {
      address: member.address ?? '',
      weight: Number(member.weight ?? 0),
      name: identity.name,
      fingerprint: identity.fingerprint,
      addedAt: member.added_at ?? null,
    };
  });

  // By name, because the chain returns them in address order and a custodian
  // looking for their own row should not have to read five bech32 strings to
  // find it. Numeric collation so chris10 sorts after chris9 rather than after
  // chris1, and unnamed members last — they are the ones worth noticing.
  return custodians.sort((a, b) => {
    if (!a.name || !b.name) return (a.name ? 0 : 1) - (b.name ? 0 : 1);
    return a.name.localeCompare(b.name, 'en', { numeric: true });
  });
}

// --------------------------------------------------------------- the group
//
// Whether the group on the chain is the group the constitution describes.

/**
 * Checks the live group against the invariants that pin it.
 *
 * This is the check that gives screen one its purpose. The constitution fixes
 * the custodian count and the signature threshold, and x/constitution's ante
 * decorator refuses transactions that would break them — but an ante decorator
 * guards changes, it does not audit the present. A group seeded at genesis with
 * four members, or a threshold policy installed as a percentage rather than a
 * count, is a state no decorator ever refused because no transaction ever
 * created it.
 *
 * So it is verified here, from two independent reads, and disagreement is
 * reported as a breach rather than resolved in favour of either side. Which of
 * the two is wrong is not something a client can know.
 */
export function auditGroup({ custodians, threshold, invariants }) {
  const findings = [];
  const expectedCount = Number(invariants?.foundation_custodian_count ?? 0);
  const expectedThreshold = Number(invariants?.foundation_signature_threshold ?? 0);

  if (!expectedCount || !expectedThreshold) {
    findings.push({
      severity: 'unknown',
      text:
        'The constitution could not be read, so nothing on this screen has been checked ' +
        'against it. Treat the membership below as unverified.',
    });
    return { ok: false, findings, expectedCount, expectedThreshold };
  }

  if (custodians.length !== expectedCount) {
    findings.push({
      severity: 'breach',
      text:
        `The group holds ${custodians.length} custodians. The constitution fixes it at ` +
        `${expectedCount}. This is a state the chain will not let a transaction create, so it ` +
        `was either seeded this way at genesis or read wrongly here — either way it is not the ` +
        `arrangement anybody attested to.`,
    });
  }

  if (threshold !== expectedThreshold) {
    findings.push({
      severity: 'breach',
      text:
        `The policy requires ${threshold} signatures. The constitution fixes it at ` +
        `${expectedThreshold}.`,
    });
  }

  // A custodian with no weight is in the group and cannot vote — which looks
  // like a five-member group and behaves like a four-member one.
  for (const c of custodians) {
    if (!(c.weight > 0)) {
      findings.push({
        severity: 'breach',
        text:
          `${c.name || c.address} carries no voting weight, so they are a custodian who cannot ` +
          `vote. The threshold is counted in weight, not in people.`,
      });
    }
    if (!c.fingerprint) {
      findings.push({
        severity: 'warn',
        text:
          `${c.name || c.address} has no key fingerprint recorded, so there is nothing here to ` +
          `check against the paper from the ceremony.`,
      });
    }
  }

  const totalWeight = custodians.reduce((s, c) => s + c.weight, 0);
  if (totalWeight && threshold && threshold * 2 <= totalWeight) {
    findings.push({
      severity: 'breach',
      text:
        `${threshold} of ${totalWeight} is not a majority. A group where two disjoint sets can ` +
        `both reach the threshold can approve two contradictory things.`,
    });
  }

  return {
    ok: findings.every((f) => f.severity === 'warn'),
    findings,
    expectedCount,
    expectedThreshold,
  };
}

// ------------------------------------------------------------ membership edits

/**
 * The member set that would result from a MsgUpdateGroupMembers.
 *
 * This mirrors `resultingMembers` in x/constitution's ante decorator, including
 * the detail that decides whether a transaction is accepted: a weight of zero
 * is a removal, any other weight is an add-or-update. The decorator then
 * requires the resulting size to equal the constitutional count exactly.
 *
 * Reimplemented here rather than approximated, because the whole purpose of
 * this console's propose screen is to never offer a shape the chain will
 * refuse. Getting this wrong means a custodian collects three signatures over a
 * week and finds out at execution.
 */
export function resultingMembers(current, updates) {
  const members = new Set(current);
  for (const update of updates ?? []) {
    const weight = Number(update.weight);
    if (!Number.isFinite(weight)) {
      throw new Error(`member ${update.address} has an unreadable weight "${update.weight}"`);
    }
    if (weight === 0) members.delete(update.address);
    else members.add(update.address);
  }
  return members;
}

/**
 * The member_updates list for replacing one custodian with another.
 *
 * Returned as one list, always, because the chain requires it: a removal on its
 * own leaves the group at four and x/constitution refuses it. The interface
 * therefore has no way to express a bare removal — not a warning against one, no
 * way to say it.
 *
 * `valid` is computed rather than assumed. The caller passes the live membership
 * and the constitutional count, and this reports whether the result satisfies
 * the rule, so the propose screen can refuse before composing a command instead
 * of after.
 */
export function swapPlan({ current, outgoing, incoming, expectedCount }) {
  const problems = [];
  const held = new Set(current);

  if (!outgoing) problems.push('Choose the custodian who is leaving.');
  else if (!held.has(outgoing)) problems.push('The departing custodian is not in this group.');

  if (!incoming) problems.push("Enter the incoming custodian's address.");
  else if (held.has(incoming)) {
    // Adding somebody already in the group is a no-op on the set, so the group
    // would shrink by one and the chain would refuse it — but the message reads
    // as a swap, which is exactly the confusion worth catching here.
    problems.push(
      'The incoming address is already a custodian, so this would remove one and add nobody.',
    );
  }
  if (incoming && incoming === outgoing) {
    problems.push('The incoming and departing addresses are the same.');
  }

  const updates =
    outgoing && incoming && outgoing !== incoming
      ? [
          { address: outgoing, weight: '0', metadata: '' },
          { address: incoming, weight: '1', metadata: '' },
        ]
      : [];

  let resulting = null;
  if (updates.length) {
    resulting = resultingMembers(current, updates).size;
    if (expectedCount && resulting !== expectedCount) {
      problems.push(
        `This would leave the group with ${resulting} custodians; the constitution fixes it at ` +
          `${expectedCount}. The chain will refuse it.`,
      );
    }
  }

  return { updates, resulting, problems, valid: problems.length === 0 };
}

// ------------------------------------------------------ what a proposal does

/**
 * One message from a proposal, in words.
 *
 * `understood` is the field that matters. A message this console cannot decode
 * is reported as not understood, and the page refuses to present it as
 * something votable — because the alternative, which is what most interfaces
 * do, is to print the type URL and the JSON and let somebody approve a transfer
 * of seized property on the strength of having seen some text.
 *
 * `concerns` carries the things that are decodable but wrong: a spend drawn on
 * an account that is not the foundation, a membership change that does not
 * balance. Those are worse than an unknown message, because they look normal.
 */
export function describeMessage(message, ctx = {}) {
  const typeUrl = message?.['@type'] ?? message?.typeUrl ?? '';
  const name = (addr) => {
    if (!addr) return 'nobody';
    const known = ctx.names?.[addr];
    return known ? `${known} (${short(addr)})` : short(addr);
  };

  if (typeUrl === '/cosmos.bank.v1beta1.MsgSend') {
    const coins = Array.isArray(message.amount) ? message.amount : [];
    const concerns = [];
    if (ctx.policyAddress && message.from_address !== ctx.policyAddress) {
      concerns.push(
        `This pays out of ${short(message.from_address)}, which is not the foundation account. ` +
          `The foundation cannot authorise a spend from an account it does not hold, so this ` +
          `will fail at execution — or it is not the proposal it appears to be.`,
      );
    }
    if (!coins.length) concerns.push('This message moves no funds at all.');
    if (!message.to_address) concerns.push('This message names no recipient.');

    return {
      typeUrl,
      understood: true,
      headline: `Pay ${formatCoins(coins, ctx)} to ${name(message.to_address)}`,
      detail: [
        { label: 'Out of', value: message.from_address ?? '', address: true },
        { label: 'To', value: message.to_address ?? '', address: true },
        ...coins.map((c) => ({ label: 'Amount', value: formatCoins([c], ctx) })),
      ],
      concerns,
      raw: message,
    };
  }

  if (typeUrl === '/cosmos.group.v1.MsgUpdateGroupMembers') {
    const updates = Array.isArray(message.member_updates) ? message.member_updates : [];
    const leaving = updates.filter((u) => Number(u.weight) === 0);
    const joining = updates.filter((u) => Number(u.weight) !== 0);
    const concerns = [];

    let resulting = null;
    if (ctx.currentMembers) {
      try {
        resulting = resultingMembers(ctx.currentMembers, updates).size;
        if (ctx.expectedCount && resulting !== ctx.expectedCount) {
          concerns.push(
            `This would leave the foundation with ${resulting} custodians; the constitution ` +
              `fixes it at ${ctx.expectedCount}. The chain will refuse it, so approving it ` +
              `achieves nothing.`,
          );
        }
      } catch (e) {
        concerns.push(`One of the member weights could not be read: ${e.message}`);
      }
    }

    // Pluralised on the number of people in each role rather than on the number
    // of edits, because a swap is two edits and one custodian: "replace X with
    // Y as custodians of the foundation" reads as though two seats changed.
    const list = (us) => us.map((u) => name(u.address)).join(' and ');
    const seat = (n) => (n === 1 ? 'a custodian' : 'custodians');
    let headline;
    if (leaving.length && joining.length) {
      headline =
        `Replace ${list(leaving)} with ${list(joining)} as ` +
        `${seat(Math.max(leaving.length, joining.length))} of the foundation`;
    } else if (leaving.length) {
      headline = `Remove ${list(leaving)} as ${seat(leaving.length)} of the foundation`;
    } else if (joining.length) {
      headline = `Add ${list(joining)} as ${seat(joining.length)} of the foundation`;
    } else {
      headline = 'Change the foundation custodians — but the message lists no changes';
    }

    return {
      typeUrl,
      understood: true,
      headline,
      detail: [
        ...leaving.map((u) => ({ label: 'Leaving', value: u.address, address: true })),
        ...joining.map((u) => ({
          label: 'Joining',
          value: `${u.address} (weight ${u.weight})`,
          address: true,
        })),
        ...(resulting === null ? [] : [{ label: 'Custodians afterwards', value: String(resulting) }]),
      ],
      concerns,
      raw: message,
    };
  }

  // A role grant or a revocation, which is what admitting a country consists of.
  //
  // Decoded here rather than only in the form that composes them, because the
  // form is not where the decision is taken. A country-enrolment proposal is
  // raised by one custodian and read by the other four on the voting screen, and
  // before this case existed it rendered there as "this console cannot say what
  // this alias message does" — for the messages that hand a national office the
  // power to freeze land or admit a bank.
  if (
    typeUrl === '/blockchain.alias.v1.MsgGrantRole' ||
    typeUrl === '/blockchain.alias.v1.MsgRevokeRole'
  ) {
    const granting = typeUrl.endsWith('MsgGrantRole');
    const known = roleByName(message.role);
    const scope = String(message.jurisdiction ?? '');
    const concerns = [];

    if (ctx.policyAddress && message.authority !== ctx.policyAddress) {
      concerns.push(
        `This names ${short(message.authority)} as the authority, which is not the foundation ` +
          'account. x/group signs every message in a proposal as the foundation, so this one is ' +
          'not authorised by it and will fail at execution.',
      );
    }
    if (!message.role || message.role === 'ROLE_UNSPECIFIED') {
      concerns.push(
        'The role is unset. The chain refuses ROLE_UNSPECIFIED everywhere, so this cannot ' +
          'execute — and proto3 cannot tell an unset field from a role numbered zero, which is ' +
          'why the enum starts at one.',
      );
    } else if (!known) {
      concerns.push(`"${message.role}" is not a role this chain has, so this cannot execute.`);
    }
    if (scope === CHAIN_WIDE) {
      concerns.push(
        'This names the chain-wide scope "*", which only governance may grant or revoke. Signed ' +
          'as the foundation it is refused before the constitution is even read, so this ' +
          'proposal would collect its signatures and then fail.',
      );
    } else if (scope === FOUNDATION_COUNTRY) {
      concerns.push(
        `"${FOUNDATION_COUNTRY}" is the foundation's reserved code, not a country. The chain ` +
          'refuses it as a grant scope: it would confer authority over nowhere while reading ' +
          'like authority over everywhere.',
      );
    } else if (!ASSIGNED_COUNTRIES.has(scope)) {
      concerns.push(
        `"${scope}" is not an assigned ISO 3166-1 alpha-2 code, so the chain will refuse this.`,
      );
    }
    if (granting && known && !known.live) concerns.push(known.caveat);
    if (granting && ctx.policyAddress && message.holder === ctx.policyAddress) {
      concerns.push(
        'The holder is the foundation itself. That is permitted — a country admitted before its ' +
          'offices exist needs an interim authority — and it is also what a mispredicted office ' +
          'address looks like, because a policy address comes from a sequence number alone.',
      );
    }

    // The shape the grant pins the office at. Read from the message rather than
    // from anything this page looked up, because what a custodian is voting on is
    // the message — and an absent requirement has to read as a sentence rather
    // than as an empty row. Silence here is the failure: an unpinned grant looks
    // exactly like a pinned one, and it lets the office vote itself to one key
    // afterwards and keep the authority.
    const pinned = granting ? shapeOfGrant(message) : null;
    if (granting) {
      if (!message.required_shape) {
        concerns.push(
          'This grant records no required shape, so nothing holds the office to the membership '
            + 'you are looking at. An office administers itself: it can vote its own threshold '
            + 'down to one signature after this passes and keep the authority, with nothing on '
            + 'the chain refusing it and nobody notified. Every grant made before the field '
            + 'existed is in that state; a new one does not have to be.',
        );
      } else if (!pinned) {
        concerns.push(
          'This grant carries a required shape this page cannot read as two whole numbers, so '
            + 'what it pins the office at is unknown. Do not vote on it here.',
        );
      } else {
        const invalid = validateShape(pinned);
        if (invalid) concerns.push(`${invalid} The chain refuses this grant.`);
      }
    }

    const where = ASSIGNED_COUNTRIES.has(scope) ? scope : `${scope || '(none given)'}`;
    return {
      typeUrl,
      understood: true,
      headline: granting
        ? `Grant ${known?.label ?? message.role ?? '(no role)'} in ${where} to ${name(message.holder)}`
          + `${pinned ? `, pinned at ${shapeRule(pinned)}` : ', with no required shape'}`
        : `Revoke ${known?.label ?? message.role ?? '(no role)'} in ${where} from ${name(message.holder)}`,
      detail: [
        { label: granting ? 'Granted to' : 'Taken from', value: message.holder ?? '', address: true },
        { label: 'Role', value: message.role ?? '(none given)' },
        { label: 'Country', value: scope || '(none given)' },
        ...(known ? [{ label: 'What consults it', value: known.consumer }] : []),
        granting
          ? {
              label: 'Office must keep',
              value: pinned
                ? `${shapeRule(pinned)} — ${pinned.signatures} of its people must sign, and it must `
                  + `keep at least ${pinned.members} member${pinned.members === 1 ? '' : 's'}. `
                  + 'Fall below either and every action this role permits is refused until the '
                  + 'office restores itself, which it can do by its own vote.'
                : 'nothing — the office may reduce itself to a single key afterwards and keep this '
                  + 'authority',
            }
          : {
              label: 'Shape removed with it',
              value: 'whatever the grant recorded. A revocation names only the triple, so what is '
                + 'being removed is the grant and its requirement together — read the grant to see '
                + 'what that was.',
            },
        { label: 'Signed by', value: message.authority ?? '', address: true },
      ],
      concerns,
      raw: message,
    };
  }

  // Deliberately not a JSON dump dressed up as a description. The page shows
  // this as a refusal to advise, and the raw message only behind a disclosure
  // that says it has not been checked.
  return {
    typeUrl,
    understood: false,
    headline: `This console cannot say what ${moduleOf(typeUrl)} message does`,
    detail: [{ label: 'Type', value: typeUrl || '(none given)' }],
    concerns: [
      'Nothing here has interpreted this message. Do not vote on it from this page — read it ' +
        'with `blockchaind q group proposal` and decode it against the module that owns it.',
    ],
    raw: message,
  };
}

/** What a whole proposal would do, and whether all of it was understood. */
export function describeProposal(proposal, ctx = {}) {
  const messages = Array.isArray(proposal?.messages) ? proposal.messages : [];
  const described = messages.map((m) => describeMessage(m, ctx));
  return {
    messages: described,
    understood: described.length > 0 && described.every((d) => d.understood),
    concerns: described.flatMap((d) => d.concerns),
    empty: described.length === 0,
  };
}

// ----------------------------------------------------------------- the vote

/**
 * Who has voted, who has not, and whether the threshold is reached.
 *
 * Counted from the votes themselves rather than from the proposal's
 * `final_tally_result`, which on an open x/group proposal is not a running
 * total — it is the zero value until something tallies it. A console that read
 * that field would show every open proposal as having no votes, which is the
 * one thing a custodian is here to find out.
 *
 * `couldStillPass` exists because a proposal can be dead before its deadline.
 * With three of five needed, three noes end it — and leaving it under "open for
 * voting" until the seventh day invites two more people to spend their week
 * waiting to vote on something already decided.
 */
export function tally({ custodians, votes, threshold, finalTally = null }) {
  const totalWeight = custodians.reduce((s, c) => s + c.weight, 0);
  const recorded = sumTally(finalTally);

  // Once a proposal is decided x/group prunes its votes but keeps the tally, so
  // there is a state where the numbers are knowable and who cast them is not.
  // Falling through to the vote list here would report a decided proposal as
  // having had no votes at all — and, worse, would list every custodian as
  // "has not voted" about a vote they did cast.
  if (!(votes ?? []).length && recorded > 0) {
    const n = (key) => Number(finalTally?.[key] ?? 0);
    const yes = n('yes_count');
    return {
      rows: custodians.map((c) => ({ custodian: c, option: null, at: null })),
      individualVotes: false,
      yes,
      no: n('no_count'),
      veto: n('no_with_veto_count'),
      abstain: n('abstain_count'),
      undecided: Math.max(0, totalWeight - recorded),
      stale: [],
      threshold,
      reached: yes >= threshold,
      couldStillPass: yes + Math.max(0, totalWeight - recorded) >= threshold,
      stillNeeded: Math.max(0, threshold - yes),
    };
  }

  const cast = new Map();
  for (const v of votes ?? []) cast.set(v.voter, v);

  const rows = custodians.map((c) => {
    const vote = cast.get(c.address);
    return {
      custodian: c,
      option: vote ? vote.option : null,
      at: vote ? vote.submit_time : null,
    };
  });

  const weightOf = (predicate) =>
    rows.filter(predicate).reduce((s, r) => s + r.custodian.weight, 0);

  const yes = weightOf((r) => r.option === YES);
  const no = weightOf((r) => r.option === NO);
  const veto = weightOf((r) => r.option === VETO);
  const abstain = weightOf((r) => r.option === ABSTAIN);
  const undecided = weightOf((r) => r.option === null);

  // Votes from addresses that are not current custodians. These exist after a
  // membership change and they do not count, so they are surfaced rather than
  // dropped: a vote that appears on the chain and not on this page looks like
  // the page is lying.
  const stale = (votes ?? []).filter((v) => !custodians.some((c) => c.address === v.voter));

  return {
    rows,
    individualVotes: true,
    yes,
    no,
    veto,
    abstain,
    undecided,
    stale,
    threshold,
    reached: yes >= threshold,
    // x/group's threshold policy counts only yes weight, so no and veto do not
    // block — they only matter by using up weight that can no longer vote yes.
    couldStillPass: yes + undecided >= threshold,
    stillNeeded: Math.max(0, threshold - yes),
  };
}

/** Total weight recorded in a chain tally, or 0 when there is none. */
function sumTally(t) {
  if (!t) return 0;
  return ['yes_count', 'no_count', 'no_with_veto_count', 'abstain_count']
    .reduce((s, k) => s + Number(t[k] ?? 0), 0);
}

// ----------------------------------------------------------- proposal state

/**
 * Where a proposal has actually got to.
 *
 * Two chain fields decide this and conflating them is the failure this console
 * exists to prevent: `status` says whether the custodians agreed, and
 * `executor_result` says whether anything happened as a result. A proposal that
 * is ACCEPTED with NOT_RUN is a decision everybody believes was carried out and
 * which has moved nothing.
 *
 * ACCEPTED with FAILURE is worse and rarer: it passed, somebody executed it,
 * and the messages failed in the block. The funds did not move and the
 * proposal cannot be run again.
 */
export function proposalState(proposal, nowSeconds) {
  const status = String(proposal?.status ?? '');
  const executed = String(proposal?.executor_result ?? '');
  const closesAt = timestampSeconds(proposal?.voting_period_end);
  const expiresAt = closesAt === null ? null : closesAt + MAX_EXECUTION_PERIOD_SECONDS;
  const open = status.endsWith('SUBMITTED');

  const base = {
    closesAt,
    expiresAt,
    votingClosed: closesAt !== null && nowSeconds >= closesAt,
    executionExpired: expiresAt !== null && nowSeconds >= expiresAt,
  };

  if (status.endsWith('WITHDRAWN')) {
    return { ...base, phase: 'withdrawn', label: 'withdrawn', tone: 'mute',
      note: 'The proposers took this back before it was decided.' };
  }
  if (status.endsWith('ABORTED')) {
    return { ...base, phase: 'aborted', label: 'void', tone: 'mute',
      note:
        'The custodians or the decision policy changed after this was proposed, so it can ' +
        'never be executed. It has to be proposed again against the group as it now stands.' };
  }
  if (status.endsWith('REJECTED')) {
    return { ...base, phase: 'rejected', label: 'rejected', tone: 'bad',
      note: 'This did not reach the threshold.' };
  }
  if (status.endsWith('ACCEPTED')) {
    if (executed.endsWith('SUCCESS')) {
      return { ...base, phase: 'executed', label: 'done', tone: 'ok',
        note: 'Approved and carried out.' };
    }
    if (executed.endsWith('FAILURE')) {
      return { ...base, phase: 'failed', label: 'passed, then failed', tone: 'bad',
        note:
          'The custodians approved this and the execution failed in the block. Nothing moved, ' +
          'and this proposal cannot be run again — it has to be proposed afresh.' };
    }
    if (base.executionExpired) {
      return { ...base, phase: 'expired', label: 'passed, now expired', tone: 'bad',
        note:
          'This was approved and never executed, and the window to execute it has closed. ' +
          'The decision is lost; it has to be proposed and voted on again.' };
    }
    return { ...base, phase: 'awaiting-exec', label: 'passed — not yet carried out', tone: 'warn',
      note:
        'The custodians have agreed. Nothing has moved yet: a passed proposal has to be ' +
        'executed, and until somebody does that this has changed nothing.' };
  }

  if (open) {
    if (base.votingClosed) {
      return { ...base, phase: 'closing', label: 'voting closed, not yet tallied', tone: 'warn',
        note:
          'The voting period has ended and the chain has not recorded an outcome yet. Executing ' +
          'it tallies the votes and, if the threshold was met, carries it out in the same step.' };
    }
    return { ...base, phase: 'open', label: 'open for voting', tone: 'warn', note: null };
  }

  return { ...base, phase: 'unknown', label: status || 'unknown', tone: 'mute',
    note: 'This console does not recognise that status.' };
}

/**
 * Whether a proposal was raised against a version of the group that no longer
 * exists, in which case it can never execute.
 *
 * x/group records the group and policy versions a proposal was submitted under,
 * and aborts it at tally time if either has moved since. Crucially it does that
 * *lazily* — the proposal's status stays SUBMITTED until somebody tallies it, so
 * a proposal that is already dead reads as open for voting for the rest of its
 * seven days. That is the shape of the trap: the custodians spend a week
 * collecting signatures on something the chain will refuse the moment it counts
 * them.
 *
 * Found by swapping a custodian on a live chain and watching an earlier proposal
 * sit there inviting votes.
 */
export function staleAgainstGroup(proposal, { groupVersion, policyVersion }) {
  const was = String(proposal?.group_version ?? '');
  const wasPolicy = String(proposal?.group_policy_version ?? '');
  const now = String(groupVersion ?? '');
  const nowPolicy = String(policyVersion ?? '');

  const reasons = [];
  if (was && now && was !== now) {
    reasons.push(
      `the custodians have changed since it was raised (it was proposed against group version ` +
        `${was}, and the group is now at ${now})`,
    );
  }
  if (wasPolicy && nowPolicy && wasPolicy !== nowPolicy) {
    reasons.push(
      `the decision policy has changed since it was raised (version ${wasPolicy}, now ` +
        `${nowPolicy})`,
    );
  }
  return { stale: reasons.length > 0, reasons };
}

/**
 * The label a proposal wears, once the votes are known.
 *
 * `proposalState` reads the chain's two status fields and cannot see the tally,
 * which leaves it calling a proposal "open for voting" when the threshold has
 * already been met — accurate about x/group's status field and misleading about
 * the only thing a custodian wants to know. The chain does not change a
 * proposal's status when the last signature arrives; it changes it when
 * somebody executes. So the label is computed from both, here.
 */
export function displayLabel(state, counted, stale = { stale: false }) {
  if (state.phase === 'open') {
    // Before anything about the votes: a proposal the chain will abort cannot
    // pass however many signatures it gathers.
    if (stale.stale) return { label: 'void — the group has changed', tone: 'bad' };
    if (counted.reached) return { label: 'agreed — not yet carried out', tone: 'brass' };
    if (!counted.couldStillPass) return { label: 'cannot pass', tone: 'bad' };
    return { label: `${counted.stillNeeded} more to go`, tone: 'warn' };
  }
  if (state.phase === 'awaiting-exec') return { label: state.label, tone: 'brass' };
  return { label: state.label, tone: state.tone };
}

/**
 * Whether executing this proposal now would do anything.
 *
 * A proposal whose threshold is already met can be executed immediately — the
 * foundation policy sets no minimum execution period, so the seven-day voting
 * period is a deadline for votes and not a delay before acting. Stating that
 * plainly matters in both directions: a console that implied a mandatory wait
 * would leave restitution sitting for a week that nothing required.
 */
export function executability(state, counted, stale = { stale: false }) {
  // Executing a stale proposal aborts it rather than carrying it out, so this
  // must not be offered as a way to make it happen.
  if (stale.stale) return { can: false, why: null };
  if (state.phase === 'awaiting-exec') {
    return { can: true, why: 'This has passed and is waiting for somebody to carry it out.' };
  }
  if (state.phase === 'open' && counted.reached) {
    return {
      can: true,
      why:
        `${counted.yes} of ${counted.threshold} needed signatures are in and this policy sets no ` +
        `waiting period, so it can be carried out now rather than at the deadline.`,
    };
  }
  if (state.phase === 'closing') {
    return { can: true, why: 'Executing it now tallies the votes and settles the outcome.' };
  }
  return { can: false, why: null };
}

// ------------------------------------------------- the ones already carried out
//
// A proposal that executes successfully is DELETED from x/group's state, along
// with every vote on it. `/cosmos/group/v1/proposal/2` returns not-found the
// moment proposal 2 has done what it was for.
//
// That is reasonable of the module and disastrous on a custody console. The
// custodian who approved a restitution comes back to check it went through and
// the proposal is simply gone — which looks exactly like a proposal that never
// existed, or one somebody removed. It is the mirror of the failure this whole
// page is built around: instead of everybody believing money moved when it did
// not, everybody has reason to doubt it moved when it did.
//
// So the record is rebuilt from the transaction log, which keeps what state
// discards. The chain remains the only source; nothing here is cached.

/** Event attribute values arrive JSON-encoded, so `"2"` is the string `"\"2\""`. */
function attr(event, key) {
  const found = (event.attributes ?? []).find((a) => a.key === key);
  if (!found) return null;
  try {
    return JSON.parse(found.value);
  } catch {
    // An older node emits them unquoted. Taking the raw value is right in that
    // case and harmless in this one.
    return found.value;
  }
}

/**
 * Executions found in the transaction log, successful or not.
 *
 * A `MsgExec` that changed nothing is still worth having: it is the difference
 * between "nobody has tried to carry this out" and "somebody tried and the
 * threshold was not met", and those call for different actions.
 */
export function parseExecutions(txResponses) {
  const out = [];
  for (const response of txResponses ?? []) {
    for (const event of response.events ?? []) {
      if (!event.type.endsWith('EventExec')) continue;
      const proposalId = attr(event, 'proposal_id');
      if (proposalId === null) continue;

      // The pruning event in the same transaction carries the final tally,
      // which is the only place it survives once the proposal is deleted.
      const pruned = (response.events ?? []).find(
        (e) => e.type.endsWith('EventProposalPruned') &&
          String(attr(e, 'proposal_id')) === String(proposalId),
      );

      out.push({
        proposalId: String(proposalId),
        result: String(attr(event, 'result') ?? ''),
        succeeded: String(attr(event, 'result') ?? '').endsWith('SUCCESS'),
        logs: String(attr(event, 'logs') ?? ''),
        height: Number(response.height ?? 0),
        time: response.timestamp ?? null,
        hash: response.txhash ?? '',
        tally: pruned ? attr(pruned, 'tally_result') : null,
        finalStatus: pruned ? String(attr(pruned, 'status') ?? '') : '',
      });
    }
  }
  return out;
}

/**
 * What each proposal contained, recovered from the transaction that raised it.
 *
 * Keyed by the proposal id from `EventSubmitProposal`, because the submitting
 * message does not carry the id — the chain assigns it. Without this join an
 * executed proposal could be reported as having happened but not as having done
 * anything, which is only half an answer.
 */
export function parseSubmissions(txs, txResponses) {
  const byId = new Map();
  (txResponses ?? []).forEach((response, i) => {
    const submitted = (response.events ?? []).find((e) => e.type.endsWith('EventSubmitProposal'));
    if (!submitted) return;
    const id = attr(submitted, 'proposal_id');
    if (id === null) return;

    const body = txs?.[i]?.body?.messages ?? [];
    const message = body.find((m) => String(m['@type'] ?? '').endsWith('MsgSubmitProposal'));
    if (!message) return;

    byId.set(String(id), {
      title: message.title ?? '',
      summary: message.summary ?? '',
      messages: message.messages ?? [],
      proposers: message.proposers ?? [],
      hash: response.txhash ?? '',
      height: Number(response.height ?? 0),
      time: response.timestamp ?? null,
    });
  });
  return byId;
}

/**
 * The proposals that were carried out, newest first.
 *
 * Only the successes. A failed or no-op execution belongs against the proposal
 * that is still in state, where the page can offer to try again — listing it as
 * history would suggest something happened.
 */
export function carriedOut(executions, submissions) {
  return executions
    .filter((e) => e.succeeded)
    .map((e) => ({ ...e, proposal: submissions.get(e.proposalId) ?? null }))
    .sort((a, b) => b.height - a.height);
}

// -------------------------------------------------------------- the commands
//
// Signing happens where the key is, which for a custodian is their own machine
// or their own paper. See the note at the top of index.html for why this
// console composes commands instead of signing in the browser.

export const CHAIN = {
  bin: 'blockchaind',
  fees: '4000uyml',
  gas: '600000',
};

/**
 * The chain id these commands are for.
 *
 * Set from the node's own `/status` rather than compiled in. A hardcoded chain
 * id is wrong on every chain but one, and the way it is wrong is the expensive
 * kind: `--chain-id` is part of what a signature covers, so a command carrying
 * the wrong one produces a transaction the node rejects with a signature error
 * — which reads as a key problem, and sends a custodian looking at their key
 * instead of at the flag. It was hardcoded here first, and a devnet with a
 * different id is precisely how that was found.
 */
let chainId = '';
export function setChainId(id) {
  chainId = String(id ?? '');
}
export function currentChainId() {
  return chainId;
}

/** Shell-quotes only when needed, so ordinary ids and addresses stay readable. */
export function sh(value) {
  const s = String(value ?? '');
  if (s !== '' && /^[A-Za-z0-9_@%+=:,.\/-]+$/.test(s)) return s;
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

function wrap(parts) {
  const lines = [parts[0]];
  for (const part of parts.slice(1)) {
    const last = lines[lines.length - 1];
    if (last.length + part.length + 1 <= 78) lines[lines.length - 1] = `${last} ${part}`;
    else lines.push(`  ${part}`);
  }
  return lines.map((l, i) => (i < lines.length - 1 ? `${l} \\` : l)).join('\n');
}

const common = (from) => [
  `--from ${sh(from)}`,
  // An empty chain id would compose a command that looks complete and cannot
  // work, so it is left visibly blank for the custodian to fill rather than
  // quietly omitted.
  `--chain-id ${sh(chainId || '<chain-id>')}`,
  `--fees ${CHAIN.fees}`,
  '--yes',
];

/**
 * Vote on a proposal.
 *
 * The proposal id and the option are filled in from what is on screen, which is
 * the entire point of composing this rather than documenting it: a custodian
 * retyping an id at the end of a long week votes on the wrong proposal, and a
 * yes on the wrong proposal is indistinguishable from a yes on the right one.
 *
 * The empty metadata positional is required by the subcommand — it takes four
 * arguments, and omitting the fourth makes it read the flags as positionals and
 * fail with a message about an address. It is quoted rather than left bare so
 * the shell still passes four words.
 *
 * `alsoExecute` adds `--exec 1`, which votes and, if that vote completes the
 * threshold, carries the proposal out in the same transaction. Offered to the
 * custodian whose vote would be the deciding one, because the alternative is a
 * proposal that passes and then waits for somebody to notice.
 */
export function voteCommand({ proposalId, voter, option, alsoExecute = false }) {
  return wrap([
    `${CHAIN.bin} tx group vote`,
    sh(proposalId),
    sh(voter),
    sh(option),
    "''",
    ...common(voter),
    `--gas ${CHAIN.gas}`,
    ...(alsoExecute ? ['--exec 1'] : []),
  ]);
}

/** Execute a proposal that has passed. Anyone may send this. */
export function execCommand({ proposalId, executor }) {
  return wrap([
    `${CHAIN.bin} tx group exec`,
    sh(proposalId),
    ...common(executor),
    `--gas ${CHAIN.gas}`,
  ]);
}

/**
 * Submit a proposal, in two steps.
 *
 * x/group takes the proposal's messages as a JSON document rather than as
 * flags, so the document is written first and submitted second. Both halves are
 * shown: a single copied line that silently depended on a file the custodian
 * never saw would fail with a path error at the point where they were trying to
 * move seized property back to somebody.
 */
export function proposalDocument({ policyAddress, proposer, messages, title, summary }) {
  return JSON.stringify(
    {
      group_policy_address: policyAddress,
      messages,
      metadata: '',
      proposers: [proposer],
      title,
      summary,
    },
    null,
    2,
  );
}

export function submitCommand({ proposer, file = 'proposal.json' }) {
  return wrap([
    `${CHAIN.bin} tx group submit-proposal`,
    sh(file),
    ...common(proposer),
    `--gas ${CHAIN.gas}`,
  ]);
}

/** The bank message for a foundation payment, as it goes into the document. */
export function spendMessage({ policyAddress, recipient, amount, denom }) {
  return {
    '@type': '/cosmos.bank.v1beta1.MsgSend',
    from_address: policyAddress,
    to_address: recipient,
    amount: [{ denom, amount: String(amount) }],
  };
}

/** The membership message for a swap, as it goes into the document. */
export function swapMessage({ policyAddress, groupId, updates }) {
  return {
    '@type': '/cosmos.group.v1.MsgUpdateGroupMembers',
    // The group administers itself, so the admin is the policy account and the
    // change can only arrive as a proposal the custodians vote on.
    admin: policyAddress,
    group_id: String(groupId),
    member_updates: updates,
  };
}

/**
 * x/group caps proposal metadata, titles and summaries at MaxMetadataLen.
 *
 * Checked before composing rather than after broadcasting, because the failure
 * is a rejected transaction whose message names a byte length and not the field
 * that was too long.
 */
export const MAX_METADATA_LEN = 255;

export function tooLong(value) {
  return new TextEncoder().encode(String(value ?? '')).length > MAX_METADATA_LEN;
}

// ------------------------------------------------------------------ helpers

/** Seconds since the epoch from an RFC3339 timestamp, or null. */
export function timestampSeconds(value) {
  if (!value) return null;
  const t = Date.parse(value);
  return Number.isFinite(t) ? Math.floor(t / 1000) : null;
}

/**
 * A duration in words, for a deadline.
 *
 * Days and hours rather than a block height, because the question a custodian
 * has is whether this decides before or after they get back on Monday.
 */
export function relativeTime(seconds) {
  const s = Math.abs(seconds);
  const past = seconds < 0;
  const say = (n, unit) => `${n} ${unit}${n === 1 ? '' : 's'}${past ? ' ago' : ''}`;
  if (s < 90) return past ? 'just now' : 'in under two minutes';
  if (s < 5400) return past ? say(Math.round(s / 60), 'minute') : `in ${say(Math.round(s / 60), 'minute')}`;
  if (s < 172800) return past ? say(Math.round(s / 3600), 'hour') : `in ${say(Math.round(s / 3600), 'hour')}`;
  return past ? say(Math.round(s / 86400), 'day') : `in ${say(Math.round(s / 86400), 'day')}`;
}

export function short(address) {
  const s = String(address ?? '');
  return s.length > 18 ? `${s.slice(0, 10)}…${s.slice(-6)}` : s;
}

function moduleOf(typeUrl) {
  const m = /^\/([^.]+)\.([^.]+)\./.exec(String(typeUrl ?? ''));
  return m ? `this ${m[2]}` : 'this';
}

/**
 * Coins as a person reads them.
 *
 * The registry maps a base denom to its display symbol and exponent. An unknown
 * denom is shown in base units with its raw name rather than divided by a
 * guessed exponent, because a wrong exponent on a restitution payment is a
 * factor of a million.
 */
export function formatCoins(coins, ctx = {}) {
  if (!coins?.length) return 'nothing';
  return coins.map((c) => formatCoin(c, ctx)).join(' and ');
}

export function formatCoin(coin, ctx = {}) {
  const info = ctx.registry?.[coin?.denom];
  const amount = String(coin?.amount ?? '0');
  if (!info) return `${groupDigits(amount)} ${coin?.denom ?? ''}`.trim();
  const exponent = Number(info.exponent ?? 0);
  if (!exponent) return `${groupDigits(amount)} ${info.symbol}`;
  const negative = amount.startsWith('-');
  const digits = (negative ? amount.slice(1) : amount).padStart(exponent + 1, '0');
  const whole = digits.slice(0, digits.length - exponent);
  const frac = digits.slice(digits.length - exponent).replace(/0+$/, '');
  const shown = `${groupDigits(whole)}${frac ? `.${frac}` : ''}`;
  return `${negative ? '-' : ''}${shown} ${info.symbol}`;
}

/**
 * Whole units to base units, by string.
 *
 * Not `value * 10 ** exponent`. A float multiplication is how 12.30 YML becomes
 * 12,299,999 base units, and this figure is the amount of somebody's recovered
 * property — it is the one number on this page where being out by one is a
 * different sum of money and being out by a factor of ten is a scandal.
 *
 * Digits past the denom's exponent are refused rather than rounded. Silently
 * dropping them would turn 0.0000005 YML into zero and 1.9999999 into 1.999999,
 * and a proposal for an amount nobody typed is worse than a form that says no.
 */
export function toBaseUnits(whole, exponent) {
  const text = String(whole ?? '').trim();
  if (!/^\d+(\.\d+)?$/.test(text)) throw new Error(`"${text}" is not a plain amount`);

  const [intPart, fracPart = ''] = text.split('.');
  if (fracPart.length > exponent) {
    throw new Error(
      exponent === 0
        ? 'this currency has no subdivisions, so it takes whole numbers only'
        : `this currency has ${exponent} decimal places and that has ${fracPart.length}`,
    );
  }

  const base = `${intPart}${fracPart.padEnd(exponent, '0')}`.replace(/^0+(?=\d)/, '');
  return base || '0';
}

/**
 * Thousands separators, inserted into the digit string itself.
 *
 * Not `toLocaleString`. Two custodians reading the same proposal must see the
 * same number: the browser's locale decides whether that function emits a
 * comma, a full stop or a narrow space, and two people comparing an amount over
 * the phone should not have to establish whose laptop is set to which country
 * before they can agree on what they are approving.
 *
 * Grouping the string rather than a Number also keeps it exact. Base units run
 * past 2^53 — a hundred million YML is 10^14 uyml, and the seizure window cap is
 * denominated in base units — and a restitution amount that has been through a
 * float is an amount nobody should sign.
 */
export function groupDigits(value) {
  const s = String(value ?? '');
  const negative = s.startsWith('-');
  const [whole, frac] = (negative ? s.slice(1) : s).split('.');
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  return `${negative ? '-' : ''}${grouped}${frac ? `.${frac}` : ''}`;
}

/**
 * The height a stalled node is stuck at, read out of its own refusal.
 *
 * A Cosmos node builds every query against the state left by the last block it
 * finalised. A node that is running but has not finalised one — because the
 * chain has halted and it was restarted, or because it is a fresh replica —
 * therefore answers *every* REST query with an error, not with stale data:
 *
 *   invalid height: context did not contain latest block height in either
 *   check state or finalize block state (2733)
 *
 * The height in the parentheses is the last committed block, and asking for it
 * explicitly through `x-cosmos-block-height` works perfectly. So the difference
 * between a console that is blank during an outage and one that still shows the
 * custodians, the balance and every open proposal is this regex.
 *
 * That difference matters most exactly when it appears. A chain halts because a
 * validator is gone, and a validator being gone is when somebody opens this page
 * to find out who can be called — which is the moment a page that reads "could
 * not read the chain" is worth nothing.
 *
 * Returns null for anything else, and null for height 0: a node that has never
 * committed a block has no state to show, and querying at height 0 means
 * "latest", which is the request that just failed.
 */
export function stalledAtHeight(body) {
  const text = typeof body === 'string' ? body : JSON.stringify(body ?? '');
  const m = /context did not contain latest block height[^)]*\((\d+)\)/.exec(text);
  if (!m) return null;
  const height = Number(m[1]);
  return Number.isSafeInteger(height) && height > 0 ? height : null;
}

/**
 * A composed command, with carriage returns removed.
 *
 * Belt and braces over the `eol=lf` pin in .gitattributes. Every command on this
 * page is built from a template literal in a source file, so the file's own line
 * endings end up inside the text a custodian copies — and a CRLF pasted into a
 * shell gives `$'\r': command not found`, an error that names neither the cause
 * nor the file. The page is served from more than one host and edited on
 * Windows, so the guarantee is worth making here rather than assuming it
 * upstream.
 */
export function shellSafe(text) {
  return String(text ?? '').replace(/\r/g, '');
}

// --- the general case ---------------------------------------------------------
//
// The foundation is a multisig account, so it can do anything an account can do:
// pay a grant, delegate a stake, vote in governance, update a module it has
// authority over. The two forms above are shortcuts for the cases that recur,
// not the limit of what the account is — and a console offering only shortcuts
// would quietly redefine the account as the two things its interface happened to
// implement.
//
// So there is a general form, and its honesty problem is the whole design
// question: this page cannot decode arbitrary protobuf, so for most messages it
// cannot tell a custodian what they are approving. It says so, loudly, rather
// than rendering a confident summary of something it has not understood. What it
// can do is check the things that are checkable without understanding the
// message at all — and those turn out to be the failures that actually happen.

/**
 * Messages pasted by a custodian, in any of the three shapes they arrive in.
 *
 * A full proposal document, a bare array, or a single message object. All three
 * are things a person reasonably pastes: the first is what this page emits and
 * what the CLI takes, the second is what a colleague sends in a chat, and the
 * third is what a module's documentation shows. Accepting one and rejecting the
 * others would be a puzzle, not a validation.
 */
export function parseMessages(text) {
  const raw = String(text ?? '').trim();
  if (!raw) return { messages: [], error: 'Nothing pasted yet.' };

  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (e) {
    // The parser's own message names the character offset, which is the most
    // useful thing anybody has ever said about broken JSON.
    return { messages: [], error: `That is not valid JSON — ${e.message}` };
  }

  const messages = Array.isArray(parsed)
    ? parsed
    : Array.isArray(parsed?.messages)
      ? parsed.messages
      : parsed && typeof parsed === 'object'
        ? [parsed]
        : null;

  if (!messages) {
    return { messages: [], error: 'Expected a message, a list of messages, or a proposal document.' };
  }
  if (!messages.length) {
    return { messages: [], error: 'That proposal contains no messages, so it would do nothing.' };
  }
  return { messages, error: null };
}

/**
 * The field names that carry the signer in the messages that have them.
 *
 * Split in two because the confidence differs and saying so is the point.
 * `from_address`, `authority` and `admin` are the signer wherever they appear in
 * the SDK's own messages — MsgSend, MsgUpdateParams, MsgUpdateGroupMembers. The
 * rest are *usually* the signer and sometimes just a party to the message, so
 * they are worth raising and not worth asserting.
 */
const SIGNER_DEFINITE = ['from_address', 'authority', 'admin'];
const SIGNER_LIKELY = [
  'sender', 'signer', 'owner', 'creator', 'proposer', 'voter', 'depositor',
  'delegator_address', 'granter', 'grantee', 'from',
];

/** Anything that looks like it might be an address, without pretending to validate one. */
const looksLikeAddress = (v) => typeof v === 'string' && /^[a-z]{2,10}1[02-9ac-hj-np-z]{20,}$/.test(v);

/**
 * What can be checked about a message without decoding it.
 *
 * Three things, and between them they cover the failures that actually reach a
 * vote:
 *
 *  1. **No type URL.** Rejected at submission, immediately, so it costs nothing
 *     but confusion — but it is the single most common paste error.
 *
 *  2. **A signer that is not the foundation.** This is the one worth having.
 *     x/group executes a proposal's messages with the *policy address* as the
 *     only signer, so a message naming anybody else is not authorised by this
 *     account. It passes submission, collects three signatures, waits out the
 *     voting period, and fails at execution — the most expensive possible place
 *     to find out, because by then several people have committed to it and the
 *     decision looks taken.
 *
 *  3. **Nothing recognised.** Reported as an absence of knowledge rather than a
 *     verdict, because "this page could not read it" and "this is fine" must
 *     never look the same.
 */
export function auditMessages(messages, { policyAddress } = {}) {
  return (Array.isArray(messages) ? messages : []).map((m, i) => {
    const typeUrl = m?.['@type'] ?? m?.typeUrl ?? '';
    const problems = [];
    const concerns = [];

    if (!typeUrl) {
      problems.push(
        'This message has no "@type", so the chain cannot tell what it is. Every message in a ' +
          'proposal needs one, e.g. "/cosmos.bank.v1beta1.MsgSend".',
      );
    } else if (!typeUrl.startsWith('/') || !typeUrl.includes('.')) {
      problems.push(
        `"${typeUrl}" is not a type URL. They start with a slash and name a proto package, ` +
          'like "/cosmos.staking.v1beta1.MsgDelegate".',
      );
    }

    if (policyAddress && m && typeof m === 'object') {
      for (const field of SIGNER_DEFINITE) {
        if (looksLikeAddress(m[field]) && m[field] !== policyAddress) {
          problems.push(
            `"${field}" is ${m[field]}, which is not the foundation account. x/group signs every ` +
              'message in a proposal as the foundation, so this one is not authorised by it: it ' +
              'would be submitted, voted on, and then fail at execution.',
          );
        }
      }
      for (const field of SIGNER_LIKELY) {
        if (looksLikeAddress(m[field]) && m[field] !== policyAddress) {
          concerns.push(
            `"${field}" is ${m[field]}, not the foundation. If that field is this message's ` +
              'signer, the proposal will fail at execution — the foundation is the only signer a ' +
              'proposal has. If it is just a party to the message, this is fine.',
          );
        }
      }
    }

    return { index: i, typeUrl, problems, concerns };
  });
}

/** A message the custodian typed, exactly as typed, for the proposal document. */
export function customMessages(messages) {
  return Array.isArray(messages) ? messages : [];
}

// --- the country-scoped roles -------------------------------------------------
//
// The foundation's other job, and the one it had no interface for at all.
//
// A country becomes operational when its offices hold roles: a lands commission
// that can register a parcel, a central bank that can admit an issuer, a payments
// authority that can admit a participant. Those grants are the foundation's act —
// three of five custodians from five organisations — and until this section
// existed the only way to make one was to hand-assemble the protobuf JSON, which
// is how a live run of the country ceremony granted a role to the foundation's
// own address instead of to the office it meant.
//
// Everything below is a refusal or a description. Nothing here signs, and nothing
// here talks to the chain: the two lookups these rules depend on — is the holder
// a group, and does the grant already exist — are performed by the page and
// passed in, so the rules themselves stay testable.

/**
 * The five roles, as role.proto declares them, and what each one actually
 * switches on today.
 *
 * `live` is the field that stops this being a list of names. Two of the five are
 * granted through the same registry as the rest and are consulted by nothing an
 * office can reach, and a console that listed all five identically would be
 * telling a custodian they had switched on an enforcement capability when they
 * had not. Granting them is still right — appointing an office later is harder
 * than granting a role that is waiting for its message — so they are offered,
 * with the gap stated at the point of signing rather than in a document nobody
 * opens.
 *
 * Kept in the proto's order, so the numbers a custodian sees here match the
 * numbers in an event attribute or a store key.
 *
 * `picker` is the one-line form, short enough to survive a `<select>` at 375px.
 * It is a separate field rather than a truncation of `consumer` because the half
 * that must not be cut is the half that says the role does nothing — a label
 * clipped to "Enforcement authority — x/enforce…" reads as a working capability.
 */
export const ROLES = [
  {
    name: 'ROLE_REGISTRY_AUTHORITY',
    number: 1,
    label: 'Registry authority',
    office: 'A lands commission or cadastral office.',
    consumer: 'x/land — registering a parcel, validating a transfer, freezing land.',
    picker: 'x/land',
    live: true,
    caveat: null,
  },
  {
    name: 'ROLE_MONETARY_AUTHORITY',
    number: 2,
    label: 'Monetary authority',
    office: 'A central bank.',
    consumer: 'x/stablecoin — approving the issuer of a currency inside this country.',
    picker: 'x/stablecoin',
    live: true,
    caveat: null,
  },
  {
    name: 'ROLE_PAYMENTS_AUTHORITY',
    number: 3,
    label: 'Payments authority',
    office: 'The office that licenses payment service providers.',
    consumer: 'x/paymsg — admitting the institutions that may appear on a payment instruction.',
    picker: 'x/paymsg',
    live: true,
    caveat: null,
  },
  {
    name: 'ROLE_ENFORCEMENT_AUTHORITY',
    number: 4,
    label: 'Enforcement authority',
    office: 'The office that can stop an account.',
    consumer: 'x/enforcement — but no message an office can send consults it yet.',
    picker: 'nothing uses it yet',
    live: false,
    caveat:
      'This role cannot be used by an office today. x/enforcement consults it in exactly two ' +
      'places and both have a second gate a group account cannot pass: MsgOpenCase requires the ' +
      'opener to be a bonded validator, which a group policy cannot be, and MsgEmergencyFreeze ' +
      "requires the signer to be x/enforcement's emergency_authority parameter. So this grant is " +
      'real, recorded and attributable, and it freezes nothing. Grant it if this is the office ' +
      'that should hold it when the gate opens — appointing an office later is much harder than ' +
      'granting a role that is waiting for its message — but do not tell anybody the country can ' +
      'now stop an account.',
  },
  {
    name: 'ROLE_SUPERVISOR',
    number: 5,
    label: 'Supervisor',
    office: 'An auditor or a regulator watching a perimeter it does not administer.',
    consumer: 'Nothing at all.',
    picker: 'nothing uses it yet',
    live: false,
    caveat:
      'Nothing on this chain consults this role. It is oversight without the power to act, and ' +
      'today it is also oversight without a single message that reads it — which is exactly what ' +
      "role.proto's own comment warns about: a role nothing consults is a name in a registry " +
      'pretending to be a control. Granting it records who is watching this country and confers ' +
      'no capability whatsoever. That is a defensible thing to want; it is not a control, and a ' +
      'custodian signing it should not believe it is one.',
  },
];

/** A role by its enum name, or null. Never a default — see ROLE_UNSPECIFIED. */
export function roleByName(name) {
  return ROLES.find((r) => r.name === name) ?? null;
}

/**
 * The scope no border bounds, which the foundation may not grant.
 *
 * Offered nowhere on this page, and refused with the reason when somebody types
 * it — because a custodian who reaches for it has understood the field correctly
 * and misunderstood who they are.
 */
export const CHAIN_WIDE = '*';

/**
 * The foundation's own reserved code. Not a country, and not chain-wide.
 *
 * Refused as a grant scope by the chain, and worth refusing here with its own
 * message rather than folding into "not a country": an operator who typed ZZ was
 * reaching for something, and the something they were reaching for is "*".
 */
export const FOUNDATION_COUNTRY = 'ZZ';

/**
 * ISO 3166-1 alpha-2, the assigned list.
 *
 * A duplicate of the table in x/alias/types/iso3166.go, and the duplication is
 * the cost of this page having no build step: it cannot read a Go table, and
 * fetching the list is not an option because the chain publishes no endpoint for
 * it. Kept in the same shape and the same order as the Go source so the two can
 * be diffed by eye when ISO next changes the list, which is every few years.
 *
 * A shape check would not do. NX, QK and ZX are all two letters and none of them
 * is a country, so a mistyped code composes a grant over a perimeter no authority
 * holds and no authority can act on — and the chain refuses it, which means the
 * mistake surfaces after three custodians have signed rather than before one has.
 */
const ASSIGNED = (
  'AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ ' +
  'BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ ' +
  'CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ ' +
  'DE DJ DK DM DO DZ EC EE EG EH ER ES ET ' +
  'FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY ' +
  'HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT ' +
  'JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ ' +
  'LA LB LC LI LK LR LS LT LU LV LY ' +
  'MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ ' +
  'NA NC NE NF NG NI NL NO NP NR NU NZ OM ' +
  'PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA ' +
  'RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ ' +
  'TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ ' +
  'UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW'
).split(' ');

export const ASSIGNED_COUNTRIES = new Set(ASSIGNED);

/** The list a picker offers, in code order. */
export function assignedCountries() {
  return [...ASSIGNED];
}

/**
 * Uppercase a country code, and leave the chain-wide marker exactly alone.
 *
 * The marker is passed through rather than folded, matching NormaliseScope, so
 * that normalisation cannot invent it: no case-folding of any two-letter code
 * produces "*", and an operator must not be able to arrive at chain-wide
 * authority by mistyping a country.
 */
export function normaliseScope(text) {
  const s = String(text ?? '').trim();
  if (s === CHAIN_WIDE) return CHAIN_WIDE;
  return s.replace(/[a-z]/g, (c) => c.toUpperCase());
}

/**
 * Whether a typed jurisdiction is one the foundation may name, and why not.
 *
 * Four refusals, each with its own sentence, because they are four different
 * mistakes:
 *
 *  1. nothing typed. A grant with no scope is not a narrower grant.
 *  2. the chain-wide marker. Not a validation error — a correctly spelled value
 *     that this account may not sign for, and saying so is the whole point.
 *  3. the foundation's reserved code. What the person reaching for ZZ actually
 *     wanted is "*", and they may not have that either.
 *  4. two letters ISO never assigned.
 */
export function checkScope(text) {
  const scope = normaliseScope(text);

  if (!scope) {
    return { scope: '', problem: 'Name the country this role is being granted in.' };
  }

  if (scope === CHAIN_WIDE) {
    return {
      scope,
      problem:
        'The foundation may not grant the chain-wide scope "*". Only governance can, and the ' +
        'split is deliberate: the foundation admitting a country and the foundation ' +
        'manufacturing authority over every country are different acts, and only the first one ' +
        'was decided. An account that could grant itself the scope no border bounds could then ' +
        'grant it to anybody, and the perimeter would be advisory. The chain refuses this from ' +
        'the foundation before it even reads the constitution — so a proposal naming "*" would ' +
        'collect three signatures, wait out the voting period, and then fail. Name a country. ' +
        'If the chain-wide scope is genuinely what is wanted, it has to go through a governance ' +
        'proposal.',
    };
  }

  if (scope === FOUNDATION_COUNTRY) {
    return {
      scope,
      problem:
        `"${FOUNDATION_COUNTRY}" is the foundation's own reserved code, not a country an office ` +
        'can be placed in. It marks the *absence* of a national perimeter, so a grant naming it ' +
        'would confer authority over nowhere while reading to a human like authority over ' +
        'everywhere — which is why the chain refuses it. Chain-wide is spelled "*", and the ' +
        'foundation may not grant that either.',
    };
  }

  if (!ASSIGNED_COUNTRIES.has(scope)) {
    return {
      scope,
      problem:
        `"${scope}" is not an ISO 3166-1 alpha-2 code ISO has assigned. Two letters is not ` +
        'enough — NX, QK and ZX are all two letters and none of them is a country — so the chain ' +
        'checks the code against the assigned list and refuses anything else. A mistyped code ' +
        'would be a perimeter no authority holds and no authority can act on.',
    };
  }

  return { scope, problem: null };
}

/**
 * A Yamale account address, checked for shape only. Nothing here validates a
 * checksum — the chain does that, and this page must not pretend to.
 *
 * The floor is 42 characters, which is what a bech32 account address on this
 * chain actually is: a three-character prefix, a separator, and 38 characters of
 * data for a 20-byte account. A group policy account is 32 bytes and comes out at
 * 62, so both real forms pass and everything shorter than either does not.
 *
 * Deliberately tighter than the `length < 39` used inline by the payment and
 * membership forms above, and the difference is worth stating rather than
 * quietly diverging: 39 admits a string that cannot be an address on this chain,
 * which is the shape of a half-pasted one. A mutation pass over this function
 * found the looser floor guarded nothing, and the field it guards here is the
 * office being handed authority over a country.
 */
export function checkAddress(text, what = 'address') {
  const addr = String(text ?? '').trim();
  if (!addr) return { address: '', problem: `Give the ${what}.` };
  if (!addr.startsWith('yml1') || addr.length < 42) {
    return {
      address: addr,
      problem: `That ${what} is not a Yamale account address. An account address is 42 ` +
        'characters and a group policy account is 62; anything shorter is a partial paste.',
    };
  }
  return { address: addr, problem: null };
}

/**
 * What the group-policy lookup on a holder means.
 *
 * The chain's own check is one call — `GroupPolicyInfo(holder)` — and it refuses
 * the holder if that call errors. This mirrors it against the REST surface, which
 * returns the same refusal as an HTTP 500 carrying a gRPC status body, so the
 * verdict has to come out of the message text rather than out of the status code:
 *
 *   {"code":2,"message":"codespace sdk code 38: not found: group policy"}
 *
 * Three verdicts and not two, because "this is not a group" and "this page could
 * not find out" must never render the same way. A page that treated an
 * unreachable node as a plain key would refuse a legitimate office; one that
 * treated it as a group would compose the grant the chain is about to refuse.
 */
export function classifyHolder({ status, body } = {}) {
  const text = typeof body === 'string' ? body : JSON.stringify(body ?? '');

  // `decoding bech32 failed` is the SDK's own prefix and covers every case on its
  // own; the rest are there for a gateway that reports the cause without it. The
  // wording is taken from live responses rather than guessed — a first version of
  // this said "checksum failed", which never matches, because the node says
  // "invalid checksum (expected 3xm8uj got 3xm8ju)".
  if (/decoding bech32 failed|invalid character|invalid separator|invalid checksum/i.test(text)) {
    return {
      verdict: 'malformed',
      groupId: null,
      problem:
        'The chain cannot read that as an address at all. Check it against the ceremony record ' +
        'character by character rather than retyping it.',
    };
  }

  if (/not found: group policy|group policy.*not found/i.test(text)) {
    return {
      verdict: 'plain-key',
      groupId: null,
      problem:
        'That address is not an x/group account, so the chain will refuse this grant. A role is ' +
        'only worth the office that holds it, and an office that is one key is one bribe — ' +
        'whatever the quorum downstream of it. Create the office as a group first, verify its ' +
        'membership and threshold against the ceremony record, and grant the role to the group ' +
        "policy address rather than to an official's own key.",
    };
  }

  if (Number(status) === 200) {
    const info = (typeof body === 'object' && body ? body.info : null) ?? null;
    if (info?.group_id) {
      return { verdict: 'group', groupId: String(info.group_id), problem: null };
    }
    return {
      verdict: 'unknown',
      groupId: null,
      problem:
        'The node answered the group-policy lookup and the answer named no group. Do not compose ' +
        'a grant on this: the check that would have refused a plain key did not run.',
    };
  }

  return {
    verdict: 'unknown',
    groupId: null,
    problem:
      `The group-policy lookup on this address did not answer (${status ?? 'no response'}), so ` +
      'this page cannot tell whether the holder is an office or one key. That is the one check ' +
      'that has to happen before three custodians sign, so nothing is composed until it does. ' +
      'It is the lookup that failed, and not necessarily the address.',
  };
}

/**
 * A count read off the chain, exactly, or null.
 *
 * Null rather than NaN or zero. A weight that cannot be read is a group this page
 * does not understand, and a zero would make it look like a member who had been
 * removed — which is the difference between a two-of-three and a two-of-two.
 */
function exactCount(value) {
  const s = String(value ?? '').trim();
  if (!/^\d+$/.test(s)) return null;
  const n = Number(s);
  return Number.isSafeInteger(n) ? n : null;
}

/**
 * The office a role is about to be granted to, as a custodian has to see it.
 *
 * This exists because an x/group policy address is derived from the group's
 * sequence number and from nothing else. It is therefore not evidence of who
 * controls the group: two chains, or one chain and one dry run, produce the same
 * address for the same sequence. A live run of the country ceremony predicted an
 * office's address and got *the foundation's own* for exactly that reason — both
 * were policy sequence 1 — and a console that showed only the address would have
 * shown a correct-looking one.
 *
 * So the address is not the check. The membership and the threshold are, and they
 * go in front of the custodian before the grant is composed rather than behind a
 * disclosure.
 */
export function officeSummary({ policy, members, foundationAddress } = {}) {
  const address = policy?.address ?? '';
  const decision = policy?.decision_policy ?? {};
  const threshold = exactCount(decision.threshold);
  const roster = (Array.isArray(members) ? members : []).map((m) => {
    const inner = m?.member ?? m ?? {};
    const identity = custodianIdentity(inner.metadata);
    return {
      address: inner.address ?? '',
      weight: exactCount(inner.weight),
      name: identity.name,
      fingerprint: identity.fingerprint,
    };
  });

  const totalWeight = roster.reduce((s, m) => (m.weight === null ? s : s + m.weight), 0);
  const unreadable = roster.filter((m) => m.weight === null);
  const concerns = [];

  // What the office IS, in the two numbers a grant's requirement is written in.
  // Computed the way the chain computes it, which is not the threshold — see the
  // note above MAX_OFFICE_MEMBERS.
  const shape = officeShapeOf({
    threshold,
    members: roster,
    decisionType: decision['@type'],
  });

  if (address && foundationAddress && address === foundationAddress) {
    concerns.push(
      'This holder is the foundation itself. The chain permits that on purpose — a country ' +
        'admitted before its offices exist needs an interim authority — but it is also exactly ' +
        'what goes wrong by accident, because an x/group policy address is derived from the ' +
        "group's sequence number and nothing else. A live run of the country ceremony predicted " +
        "an office's address and got the foundation's own, because both were policy sequence 1. " +
        'If this is meant to be a national office, the address is wrong.',
    );
  }
  if (policy && policy.admin && address && policy.admin !== address) {
    concerns.push(
      `This group's admin is ${short(policy.admin)}, not the group itself. That account can ` +
        'rewrite the membership without a vote, so the threshold below is advisory and the real ' +
        'holder of this role is whoever controls that admin. Granting authority to a group ' +
        'somebody outside can reconstitute is granting it to that outsider.',
    );
  }
  if (threshold !== null && threshold < 2) {
    concerns.push(
      `This group decides on ${threshold} signature${threshold === 1 ? '' : 's'}. It is an ` +
        'x/group account, so the chain will accept it, and it is not an office: a single member ' +
        'can act alone. That is the failure the group requirement exists to prevent, wearing a ' +
        "group's clothes.",
    );
  }
  if (threshold !== null && totalWeight && threshold > totalWeight) {
    concerns.push(
      `This group needs ${threshold} of a total weight of ${totalWeight}, so it can never reach ` +
        'its own threshold. Nothing it is granted can ever be used.',
    );
  }
  if (unreadable.length) {
    concerns.push(
      `${unreadable.length} member weight${unreadable.length === 1 ? '' : 's'} could not be read, ` +
        'so the threshold cannot be checked against them.',
    );
  }
  if (!roster.length) {
    concerns.push('The chain returned no members for this group, so there is nothing to inspect.');
  }
  // The weighted case, which the threshold check above cannot see. A threshold of
  // three over weights of 3, 1, 1, 1, 1 is a one-of-five: the heaviest member
  // reaches it alone. This is the sentence that stops a custodian reading "3 of
  // 5" off the panel and pinning a grant at a shape one person satisfies.
  if (shape.signatures !== null && threshold !== null && threshold >= 2 && shape.signatures < 2) {
    concerns.push(
      `This group's threshold is ${threshold}, but its member weights are not equal: `
        + `${shape.signatures === 1 ? 'one member' : `${shape.signatures} members`} can reach that `
        + 'threshold alone. It is a one-of-many wearing the clothes of a larger office, and it is '
        + 'what the chain will measure this grant against — not the threshold.',
    );
  }
  if (shape.problem) {
    concerns.push(
      `The shape of this office cannot be established: ${shape.problem}. A grant to it cannot be `
        + 'pinned, and an unpinned grant is one the office can keep after voting itself down to a '
        + 'single key.',
    );
  }

  return {
    address,
    groupId: policy?.group_id ? String(policy.group_id) : null,
    threshold,
    totalWeight,
    // The two numbers a required shape is written in, and the rule as it is said
    // out loud: "3-of-5".
    shape,
    rule: shapeRule(shape),
    // Sorted by name so two custodians comparing the same office over the phone
    // read it in the same order, whatever order the chain returned it in.
    members: roster.slice().sort((a, b) => (a.name ?? a.address).localeCompare(b.name ?? b.address)),
    concerns,
  };
}

// --- the shape an office has to keep -----------------------------------------
//
// A role grant can pin the M-of-N of the office that holds it, and the chain
// re-checks that pin on every action the grant permits — so an office that votes
// itself down to one key loses the authority automatically instead of keeping the
// power to freeze accounts. The field is `required_shape` on MsgGrantRole and on
// the stored RoleGrant.
//
// Three things about it are load-bearing enough to be encoded here rather than
// left to whoever fills in the form.
//
// SIGNATURES ARE PEOPLE, NOT THE THRESHOLD. x/group counts weight, so a policy
// with a threshold of 3 over members weighing 3, 1, 1, 1 and 1 is a one-of-five:
// the first member reaches the threshold alone. The chain takes the members in
// descending weight and counts how few can reach the threshold, and this file
// does the same arithmetic — because a console that displayed the threshold as
// "3 signatures" would show a 3-of-5 for what is one key, on the screen whose
// entire job is to stop that.
//
// ABSENT IS NOT ZERO. The field is a message so that "no requirement" and "a
// requirement of nothing" can never be confused: no requirement is the field
// omitted entirely, and a shape of zero signatures is refused by the chain. Every
// grant made before the field existed carries no requirement, and this page says
// so in words rather than rendering silence.
//
// A REQUIREMENT ONLY RATCHETS. Re-granting the same triple with a lower
// requirement, or with none, is refused: the obvious resubmission is composed
// from a summary rather than from the stored grant, leaves the field out, and
// silently removes the pin under a proposal whose stated purpose was to change
// nothing. Relaxing one deliberately is a revoke and a grant in the same
// proposal.

/** The largest office whose shape the chain will read. Mirrors types.MaxOfficeMembers. */
export const MAX_OFFICE_MEMBERS = 50;

/**
 * How few members can reach the threshold between them.
 *
 * The greedy answer is the exact one: for any k, the largest sum k members can
 * reach is the sum of the k largest weights, so sorting descending and
 * accumulating finds the minimum count rather than an estimate. Same reasoning,
 * and the same result, as the keeper's `fewestSigners`.
 *
 * A group whose threshold exceeds its total weight is frozen rather than weak,
 * and that is a different sentence: it can take no action at all, so nothing
 * turns on whether its shape is adequate.
 */
export function fewestSigners(weights, threshold) {
  const usable = (Array.isArray(weights) ? weights : []).filter((w) => Number.isFinite(w) && w > 0);
  if (!Number.isFinite(threshold) || threshold <= 0) {
    return { signatures: null, problem: 'this group records no usable threshold' };
  }
  const sorted = usable.slice().sort((a, b) => b - a);
  let total = 0;
  for (let i = 0; i < sorted.length; i += 1) {
    total += sorted[i];
    if (total >= threshold) return { signatures: i + 1, problem: null };
  }
  return {
    signatures: null,
    problem: `its ${sorted.length} members hold ${total} of voting weight between them and its `
      + `threshold is ${threshold}, so no set of members can act at all`,
  };
}

/**
 * What an office is, right now, in the two numbers a requirement is written in.
 *
 * `members` counts only members holding a positive weight: a member who cannot
 * vote is a name on a list, and padding a group with weightless members is the
 * obvious way to satisfy a count while shrinking the number of people who decide.
 *
 * A percentage decision policy is refused rather than converted, exactly as the
 * keeper refuses one. The arithmetic is possible and the meaning is not: a
 * percentage makes the threshold FOLLOW the membership, so an office could drop
 * from five members to two, still satisfy its percentage, and the number of
 * people who decide would have gone from three to two.
 */
export function officeShapeOf({ threshold, members, decisionType } = {}) {
  const roster = Array.isArray(members) ? members : [];
  const positive = roster.filter((m) => m && Number.isFinite(m.weight) && m.weight > 0);
  const unreadable = roster.filter((m) => m && m.weight === null);

  if (decisionType && !String(decisionType).includes('ThresholdDecisionPolicy')) {
    return {
      signatures: null,
      members: positive.length,
      problem: 'this group uses a percentage decision policy, and a shape cannot be held against '
        + 'one: the threshold moves with the membership, so the office could shed members and '
        + 'satisfy it forever',
    };
  }
  if (unreadable.length) {
    return {
      signatures: null,
      members: positive.length,
      problem: `${unreadable.length} member weight${unreadable.length === 1 ? '' : 's'} could not `
        + 'be read, so how many people it takes to act cannot be established',
    };
  }
  if (roster.length > MAX_OFFICE_MEMBERS) {
    return {
      signatures: null,
      members: positive.length,
      problem: `this group has ${roster.length} members, more than the ${MAX_OFFICE_MEMBERS} the `
        + 'chain will read a shape from, so it cannot hold a pinned grant at all',
    };
  }

  const fewest = fewestSigners(positive.map((m) => m.weight), threshold);
  return { signatures: fewest.signatures, members: positive.length, problem: fewest.problem };
}

/**
 * A shape as the people who agreed it say it out loud.
 *
 * "3-of-5", because that is what is on the ceremony record and said in the room.
 * Null renders as words rather than as "0-of-0": a grant with no requirement is a
 * different thing from one requiring nothing, and the whole point of the field
 * being a message is that those two never read the same.
 */
export function shapeRule(shape) {
  if (!shape || shape.signatures === null || shape.signatures === undefined) {
    return 'no required shape';
  }
  return `${shape.signatures}-of-${shape.members}`;
}

/** Floors on both numbers: an office may grow, and may not fall below its grant. */
export function shapeSatisfies(want, have) {
  if (!want) return true;
  if (!have || have.signatures === null || have.members === null) return false;
  return have.signatures >= want.signatures && have.members >= want.members;
}

/**
 * Whether a requirement is one the chain will accept at all.
 *
 * The zero case is the one worth stating: a requirement of no signatures reads on
 * a record as though it covered something, so the chain refuses it and the way to
 * record no requirement is to omit the field.
 */
export function validateShape(shape) {
  if (!shape) return null;
  const { signatures, members } = shape;
  if (!Number.isInteger(signatures) || !Number.isInteger(members)) {
    return 'A required shape has to be two whole numbers.';
  }
  if (signatures === 0) {
    return `A required shape of ${shapeRule(shape)} asks for no signatures. The chain refuses that: `
      + 'to record no requirement, choose "record no required shape" instead of asking for zero.';
  }
  if (signatures < 0 || members < 0) return 'A required shape cannot be negative.';
  if (members < signatures) {
    return `A required shape of ${shapeRule(shape)} asks for more signatures than members, which no `
      + 'office could ever satisfy.';
  }
  if (members > MAX_OFFICE_MEMBERS) {
    return `A required shape of ${shapeRule(shape)} asks for more than the ${MAX_OFFICE_MEMBERS} `
      + "members the chain can read a group's shape from.";
  }
  return null;
}

/** The shape a stored grant records, or null where it records none. */
export function shapeOfGrant(grant) {
  const raw = grant?.required_shape;
  if (!raw) return null;
  const signatures = exactCount(raw.signatures);
  const members = exactCount(raw.members);
  if (signatures === null || members === null) return null;
  return { signatures, members };
}

/**
 * The grant message, as it goes into the proposal document.
 *
 * `requiredShape` is omitted from the message entirely when it is null, and that
 * is the difference between "no requirement" and "a requirement of nothing" — see
 * the note above `MAX_OFFICE_MEMBERS`. Never send zeros.
 */
export function grantRoleMessage({ policyAddress, holder, role, jurisdiction, requiredShape = null }) {
  return {
    // The proto package is blockchain.alias.v1. Not yamale.blockchain.alias.v1,
    // which is what the module's *REST* paths are prefixed with and what anybody
    // reading those paths would reasonably guess — the two differ, and a type URL
    // the interface registry cannot resolve fails at submission with a complaint
    // about an unregistered type rather than about a typo.
    '@type': '/blockchain.alias.v1.MsgGrantRole',
    // x/group signs every message in a proposal as the policy account, so this is
    // the foundation and can be nothing else.
    authority: policyAddress,
    holder,
    // The enum's name, not its number. Proto3 JSON accepts both, and the name is
    // what the chain emits in its own events and what the reference documentation
    // spells — so a custodian diffing this document against a query result is
    // comparing like with like.
    role,
    jurisdiction,
    // Spread rather than set, so the key is absent and not null. A JSON null in
    // this position is decoded by the chain as an unset message field today, and
    // relying on that would be relying on a coincidence: the whole reason the
    // field is a message is that presence carries meaning.
    ...(requiredShape
      ? { required_shape: { signatures: requiredShape.signatures, members: requiredShape.members } }
      : {}),
  };
}

/**
 * The revocation message. The same four fields, and that is not an oversight.
 *
 * A revocation names the whole triple because a holder may hold the same role in
 * several countries, and revoking "their enforcement role" would be ambiguous
 * between removing one perimeter and removing all of them. The signer says which.
 */
export function revokeRoleMessage({ policyAddress, holder, role, jurisdiction }) {
  return {
    '@type': '/blockchain.alias.v1.MsgRevokeRole',
    authority: policyAddress,
    holder,
    role,
    jurisdiction,
  };
}

/** One of a holder's grants, by role and jurisdiction, or null. */
export function findGrant(grants, role, jurisdiction) {
  const list = Array.isArray(grants) ? grants : [];
  return list.find((g) => g?.role === role && g?.jurisdiction === jurisdiction) ?? null;
}

/**
 * Everything that has to be true before a grant is composed.
 *
 * `problems` blocks; `concerns` is said and signed anyway. The line between them
 * is whether the chain would refuse it, with exceptions in both directions that
 * are deliberate and stated where they are raised:
 *
 *   - the chain-wide scope and a plain-key holder are refusals the chain makes,
 *     mirrored here so they cost a form field rather than a voting period;
 *   - a 1-of-N office, a group somebody else administers, and a holder that is
 *     the foundation itself are all things the chain accepts and a room should
 *     probably not. They are concerns rather than refusals, because this page
 *     cannot know a deployment's intent, and a console that refused them would
 *     be a console somebody routed around through the general form — where none
 *     of this is checked at all.
 */
export function grantPlan({
  policyAddress,
  holder,
  role,
  jurisdiction,
  holderVerdict = null,
  office = null,
  existingGrants = null,
  // The M-of-N the office must keep to keep the role, or null for no
  // requirement. Null is a real choice here rather than a default: a grant with
  // no requirement is one the office can keep after voting itself to one key,
  // and the form has to say so out loud rather than omit a field quietly.
  requiredShape = null,
}) {
  const problems = [];
  const concerns = [];

  const addr = checkAddress(holder, 'address of the office being granted this role');
  if (addr.problem) problems.push(addr.problem);

  const known = roleByName(role);
  if (!role) {
    problems.push('Choose the role being granted.');
  } else if (!known) {
    problems.push(
      `"${role}" is not a role this chain has. ROLE_UNSPECIFIED is refused everywhere: proto3 ` +
        'cannot tell a zero from a field nobody filled in, so a grant whose role was left unset ' +
        'must never be honoured.',
    );
  }

  const scope = checkScope(jurisdiction);
  if (scope.problem) problems.push(scope.problem);

  // The group check comes last of the blocking ones, because the three above are
  // answerable without asking the chain anything and this one is not.
  if (!addr.problem) {
    if (!holderVerdict) {
      problems.push(
        'Waiting on the group-policy lookup for this address. Nothing is composed until the ' +
          'chain has confirmed the holder is an office and not one key.',
      );
    } else if (holderVerdict.verdict !== 'group') {
      problems.push(holderVerdict.problem);
    }
  }

  if (known && !known.live) concerns.push(known.caveat);
  for (const c of office?.concerns ?? []) concerns.push(c);

  if (existingGrants && known && !scope.problem) {
    const already = findGrant(existingGrants, role, scope.scope);
    if (already) {
      concerns.push(
        `This holder already holds ${role} in ${scope.scope}, granted by ` +
          `${short(already.granted_by ?? '')} at height ${already.granted_at_height ?? '?'}. ` +
          'Granting it again is not an error and is not a second grant: it rewrites who granted ' +
          'it and when, and changes nothing else. That is the right thing for a proposal ' +
          'resubmitted after a timeout, and pointless otherwise.',
      );
      // The ratchet, mirrored from the keeper. Re-granting the same triple with a
      // weaker requirement — or with none — is refused there, and the reason it
      // has to be refused here too is that the omission is the natural mistake: a
      // resubmission composed from a summary rather than from the stored grant
      // leaves the field out, and a proposal whose stated purpose was to change
      // nothing silently unpins an office.
      const pinned = shapeOfGrant(already);
      if (pinned && !requiredShape) {
        problems.push(
          `${short(addr.address)} already holds ${role} in ${scope.scope} pinned at `
            + `${shapeRule(pinned)}, and this grant would record no requirement. The chain refuses `
            + 'that: omitting the shape on a re-grant would remove the pin. To relax it '
            + 'deliberately, revoke the grant and make a new one in the same proposal — two '
            + 'messages, so that unpinning an office is a thing somebody wrote down.',
        );
      } else if (pinned && requiredShape && !shapeSatisfies(pinned, requiredShape)) {
        problems.push(
          `${short(addr.address)} already holds ${role} in ${scope.scope} pinned at `
            + `${shapeRule(pinned)}, and this would reduce it to ${shapeRule(requiredShape)}. A `
            + 'requirement only ratchets upward; to lower it, revoke the grant and make a new one '
            + 'in the same proposal.',
        );
      }
    }
  }

  // The requirement itself: shaped so the chain will accept it, and reachable by
  // the office as it stands today. The chain checks the second at grant time as
  // well as on every action, so a requirement the office does not meet is a
  // proposal that collects three signatures and then fails at execution.
  const shapeProblem = validateShape(requiredShape);
  if (shapeProblem) {
    problems.push(shapeProblem);
  } else if (requiredShape && office) {
    if (office.shape?.problem) {
      problems.push(
        `This grant would be pinned at ${shapeRule(requiredShape)}, and the office's own shape `
          + `cannot be established: ${office.shape.problem}. The chain checks the requirement `
          + 'against the office before it writes the grant, so it would refuse this — and a pin '
          + 'that cannot be checked is not a pin.',
      );
    } else if (!shapeSatisfies(requiredShape, office.shape)) {
      problems.push(
        `A grant pinned at ${shapeRule(requiredShape)} cannot be made to this office, which is `
          + `${shapeRule(office.shape)} today. Either the address is not the office this grant was `
          + `meant for, or the office has to be brought up to ${shapeRule(requiredShape)} before it `
          + 'can hold the authority. The chain refuses this at grant time, not at first use.',
      );
    }
  }
  if (!requiredShape && !shapeProblem) {
    concerns.push(
      'This grant records NO required shape, so nothing holds the office to its membership. An '
        + 'office administers itself: the members below can vote their own threshold down to one '
        + 'signature at any time afterwards and keep this authority, and nothing on the chain '
        + 'would refuse it or notify anybody. That is the state every grant made before the field '
        + 'existed is in. Pin it unless you mean that.',
    );
  }
  if (requiredShape && requiredShape.signatures === 1) {
    concerns.push(
      `A requirement of ${shapeRule(requiredShape)} is satisfied by one person signing, so it pins `
        + 'the office at something a single key already meets. It is better than no requirement '
        + 'only in that it stops the office shedding members.',
    );
  }

  const ready = problems.length === 0;
  return {
    ready,
    problems,
    concerns,
    role: known,
    scope: scope.scope,
    holder: addr.address,
    requiredShape: requiredShape ?? null,
    messages: ready
      ? [grantRoleMessage({
          policyAddress,
          holder: addr.address,
          role,
          jurisdiction: scope.scope,
          requiredShape: requiredShape ?? null,
        })]
      : [],
  };
}

/**
 * Everything that has to be true before a revocation is composed.
 *
 * Two differences from a grant, and both are why this is a separate function
 * rather than a flag:
 *
 * **The holder is not checked against x/group.** The chain does not check it on a
 * revocation either, and it must not: a grant that somehow reached a plain key —
 * seeded at genesis, or written while the group keeper was not wired — is
 * precisely the grant that most needs removing, and a console that demanded the
 * holder be a group would make the bad grant the one grant nobody could revoke.
 *
 * **The grant has to exist.** The chain refuses a revocation naming one that was
 * never made, deliberately, because "nothing to revoke" is how a proposal that
 * named the wrong country succeeds while leaving the authority it meant to remove
 * in place. Refusing here is that rule paid before the voting period instead of
 * after it.
 */
export function revokePlan({
  policyAddress,
  holder,
  role,
  jurisdiction,
  existingGrants = null,
}) {
  const problems = [];
  const concerns = [];

  const addr = checkAddress(holder, 'address of the office losing this role');
  if (addr.problem) problems.push(addr.problem);

  const known = roleByName(role);
  const scope = checkScope(jurisdiction);

  // A revocation names a grant, and a grant is a role and a country together — so
  // an unset pair is one mistake and gets one sentence. Reporting both halves
  // separately would ask a custodian to "name the country this role is being
  // granted in" on the form that revokes one, which is two complaints about a
  // choice they have simply not made yet.
  if (!role && !scope.scope) {
    problems.push('Choose which grant to remove.');
  } else {
    if (!role) {
      problems.push('Choose which grant to remove.');
    } else if (!known) {
      problems.push(
        `"${role}" is not a role this chain has. "Revoke whatever role was left unset" has no ` +
          'meaning, and a message that resolved it to one would revoke something nobody named.',
      );
    }
    if (scope.problem) problems.push(scope.problem);
  }

  let found = null;
  if (existingGrants === null) {
    if (!addr.problem) {
      problems.push(
        "Waiting on this holder's grants. A revocation naming a grant that was never made is " +
          'refused by the chain, so nothing is composed until the chain has listed what there ' +
          'is to remove.',
      );
    }
  } else if (known && !scope.problem && !addr.problem) {
    found = findGrant(existingGrants, role, scope.scope);
    if (!found) {
      const held = existingGrants.length
        ? `What it does hold: ${existingGrants
            .map((g) => `${g.role} in ${g.jurisdiction}`)
            .join(', ')}.`
        : 'This holder holds no role grants at all.';
      problems.push(
        `${short(addr.address)} does not hold ${role} in ${scope.scope}, so there is nothing ` +
          `there to revoke and the chain will refuse this. ${held}`,
      );
    }
  }

  if (found && found.granted_by && policyAddress && found.granted_by !== policyAddress) {
    concerns.push(
      `This grant was made by ${short(found.granted_by)}, not by the foundation. The foundation ` +
        "may still remove it — whoever may appoint a country's authority may also remove it, and " +
        'that includes a grant governance made. It is a real reduction of the validator set\'s ' +
        'power and it is kept on purpose, because the alternative makes an emergency the ' +
        'expensive case. Be sure this is a compromised or abused office and not a disagreement ' +
        'with a vote.',
    );
  }

  const ready = problems.length === 0;
  return {
    ready,
    problems,
    concerns,
    role: known,
    scope: scope.scope,
    holder: addr.address,
    grant: found,
    messages: ready
      ? [revokeRoleMessage({
          policyAddress,
          holder: addr.address,
          role,
          jurisdiction: scope.scope,
        })]
      : [],
  };
}
