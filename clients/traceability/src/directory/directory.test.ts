import assert from 'node:assert/strict';
import { test } from 'node:test';

import { safeContact } from '../api.ts';
import { countryNamer, fold, matches, sortListed, telLink, whatsappLink } from './directory.ts';

const diop = { id: '2', name: 'Maison Diop', contact: { city: 'Thiès', country: 'SN' } };
const atelier = { id: '1', name: 'Atelier Nour', contact: { city: 'Casablanca', country: 'MA' } };

test('a search matches name, city and country, without accents or case', () => {
  const names = countryNamer('fr');
  assert.equal(fold('Thiès'), 'thies');
  assert.ok(matches(diop, 'thies', names));
  assert.ok(matches(diop, 'diop SN', names));
  assert.ok(matches(diop, 'sénégal', names));
  assert.ok(!matches(diop, 'casablanca', names));
  assert.ok(matches(atelier, '', names));
});

test('shops are listed by name, with a stable tie-break', () => {
  assert.deepEqual(sortListed([diop, atelier, { id: '0', name: 'atelier nour' }]).map((s) => s.id), ['0', '1', '2']);
});

test('phone links keep only what the dialler and WhatsApp accept', () => {
  assert.equal(telLink('+221 33 821-00.00'), 'tel:+221338210000');
  assert.equal(whatsappLink('+221 (77) 000 00 00'), 'https://wa.me/221770000000');
});

test('contact details from the API are kept only where they are safe to show and link', () => {
  assert.deepEqual(safeContact({
    city: ' Dakar ', website: 'javascript:alert(1)', phone: 'call me', email: 'a@b.sn', country: 'sn', extra: 'x', hours: 3,
  }), { city: 'Dakar', email: 'a@b.sn' });
  assert.equal(safeContact({ city: '  ' }), undefined);
  assert.equal(safeContact(null), undefined);
  assert.deepEqual(safeContact({ website: 'https://maison-diop.sn', whatsapp: '+221 77 000 00 00' }),
    { website: 'https://maison-diop.sn', whatsapp: '+221 77 000 00 00' });
});
