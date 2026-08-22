import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { getLocale, setLocale } from './i18n.ts';
import {
  countryName,
  countryProblem,
  placementRequest,
  placementVerdict,
} from './placement.ts';

const ADDR = 'yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg';

test('an unplaced account is told the consequence, not just the status', () => {
  const v = placementVerdict({ country: null, userId: null });
  assert.equal(v.state, 'unplaced');
  // The point of the whole module: "no country" is abstract, "nobody can pay
  // you" is not.
  assert.match(v.consequence, /nobody can address a payment to you/);
  // And it must never suggest the holder can fix it themselves.
  assert.match(v.remedy, /institution that onboarded you/);
  assert.doesNotMatch(v.remedy, /you can record|set your own/i);
});

test('a placed account says where, and that it can now be paid', () => {
  const v = placementVerdict({ country: 'SN', userId: 'SN-K3M9-7QRT-B' });
  assert.equal(v.state, 'placed');
  assert.match(v.headline, /SN/);
  assert.match(v.consequence, /can be addressed/);
});

test('a country with no identifier is reported as a fault, not as placed', () => {
  // The chain issues an identifier when it records a country, so these
  // disagreeing means issuance failed. Calling it "placed" would hide a real
  // fault behind a green tick; calling it "unplaced" would send somebody back
  // to an institution that has already done its part.
  const v = placementVerdict({ country: 'GH', userId: null });
  assert.equal(v.state, 'inconsistent');
  assert.match(v.consequence, /not a normal state/);
});

test('a lowercase country is read as the same country', () => {
  assert.equal(placementVerdict({ country: 'ke', userId: 'KE-1' }).state, 'placed');
  assert.match(placementVerdict({ country: 'ke', userId: 'KE-1' }).headline, /KE/);
});

test('the foundation code is refused for an ordinary account', () => {
  // ZZ marks the absence of a national perimeter. Recorded against an ordinary
  // account it would issue an identifier that reads as chain-wide authority.
  const p = countryProblem('ZZ');
  assert.match(p!, /reserved code/);
  assert.match(p!, /not a country an account can be placed in/);
});

test('an unassigned code is refused before a request is composed', () => {
  assert.match(countryProblem('XK')!, /not an assigned ISO 3166-1/);
  assert.match(countryProblem('QQ')!, /not an assigned/);
  assert.match(countryProblem('S')!, /two letters/);
  assert.match(countryProblem('')!, /Choose the country/);
  assert.equal(countryProblem('SN'), null);
  assert.equal(countryProblem('gh'), null, 'case must not decide validity');
});

test('a country name is localised, with the code always available', () => {
  // Locales passed explicitly. countryName() with no argument follows the
  // interface's own language, and asserting against that would make this test
  // depend on whatever an earlier test in the same process last set.
  assert.equal(countryName('SN', 'en'), 'Senegal');
  assert.equal(countryName('SN', 'fr'), 'Sénégal');
  assert.equal(countryName('SN', 'pt'), 'Senegal');
  // Unknown or malformed falls back to what the chain actually stores, which is
  // the code — never a guess and never blank.
  assert.equal(countryName('QQ', 'en'), 'QQ');
  assert.equal(countryName('', 'en'), '');
});

test('a country name follows the interface language by default', () => {
  // The reason this exists: a French page reading "SN (Senegal)" is the kind of
  // half-translation that tells somebody the product was not built for them.
  const previous = getLocale();
  try {
    setLocale('fr');
    assert.equal(countryName('SN'), 'Sénégal');
    setLocale('en');
    assert.equal(countryName('SN'), 'Senegal');
  } finally {
    setLocale(previous);
  }
});

test('a placement request carries the address inside the document', () => {
  const r = placementRequest({
    address: ADDR, country: 'SN', institution: 'Banque Nationale', chainId: 'yamale-devnet-2',
  });
  assert.ok(!('problem' in r));
  const req = r as Exclude<typeof r, { problem: string }>;

  // Repeated inside the document rather than left to a covering message: being
  // placed at the wrong address succeeds, looks right, and issues an identifier
  // to an account nobody holds.
  assert.match(req.document, new RegExp(ADDR));
  assert.match(req.document, /SN \(Senegal\)/);
  assert.match(req.document, /Banque Nationale/);
  assert.match(req.document, /yamale-devnet-2/);
  // It must state the two things the holder is agreeing to.
  assert.match(req.document, /cannot record it myself/);
  assert.match(req.document, /no user ID until it is recorded/);

  assert.match(req.command, /tx alias set-jurisdiction/);
  assert.match(req.command, new RegExp(`set-jurisdiction ${ADDR} SN`));
  assert.match(req.command, /--chain-id yamale-devnet-2/);
  // The command must not imply the holder signs it.
  assert.match(req.command, /the participant's key/);
});

test('a request for a country the chain would refuse is not composed at all', () => {
  for (const bad of ['ZZ', 'XK', '', 'S']) {
    const r = placementRequest({ address: ADDR, country: bad });
    assert.ok('problem' in r, `${bad} must not produce a request`);
  }
});

test('a request with no address is refused', () => {
  const r = placementRequest({ address: '   ', country: 'SN' });
  assert.ok('problem' in r);
  assert.match((r as { problem: string }).problem, /No account address/);
});

test('an unnamed institution leaves a visible blank rather than a plausible one', () => {
  const r = placementRequest({ address: ADDR, country: 'GH' }) as { document: string };
  assert.match(r.document, /\(name the institution\)/);
});
