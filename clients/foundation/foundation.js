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
