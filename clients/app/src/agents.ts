/**
 * Cash agents — the shops where digital money becomes physical money.
 *
 * This is the half of a payments network that software people forget. A
 * remittance is only finished when somebody can eat: the recipient needs notes
 * in hand, and in most of the places this chain is built for that means walking
 * to a shop, not tapping a bank card. So the app has to answer "who near me
 * will hand me cash for this?" — and answer it offline-first, because the
 * person asking is often on a weak connection.
 *
 * The list below is **dummy data for the proof of concept**. In production this
 * belongs on-chain: an agent registers, posts a bond, and is rated by completed
 * settlements, so a fraudulent agent can be slashed and delisted. Written here
 * as plain data with that migration in mind — the shape is what a chain query
 * would return.
 */

/**
 * How a shop delivers fiat.
 *
 * A shop is not one rail. The same counter often hands over notes, tops up a
 * mobile-money wallet and pushes to a bank account, at three different prices —
 * so the price belongs to the method, not to the shop. A beneficiary choosing
 * between walking out with cash and waiting a day for a bank transfer is
 * choosing between two different fees, and hiding that behind one shop-level
 * number is how a corridor looks cheaper than it is.
 */
export type PayoutMethod = 'cash' | 'card' | 'bank' | 'momo';

export interface Payout {
  /** ISO code of the money that actually arrives — XOF, NGN, KES. */
  fiat: string;
  method: PayoutMethod;
  /** On top of the shop's own fee. Zero where the method costs nothing extra. */
  feeBps: number;
}

export interface Agent {
  id: string;
  name: string;
  /** Street-level description a person can actually navigate to. */
  address: string;
  city: string;
  country: string;
  lat: number;
  lon: number;
  /**
   * The crypto this agent takes in — for money out, what the beneficiary sends
   * them; for money in, what they credit to an account.
   *
   * Separate from `payouts` because they answer different questions and
   * conflating them was the original mistake. "Accepts USDC" and "hands over
   * CFA francs" are not the same fact, and a beneficiary holding EURC needs to
   * know which of the two the shop can actually do.
   */
  accepts: string[];
  /** The fiat this agent can put in a hand, on a card, or into a phone. */
  payouts: Payout[];
  /** What the agent charges, in basis points of the amount. */
  feeBps: number;
  /** Ceiling per settlement, in whole units of the payout fiat. */
  maxPayout: number;
  /** Completed settlements — the only reputation that costs something to fake. */
  settlements: number;
  hours: string;
  phone: string;
}

export const AGENTS: Agent[] = [
  {
    id: 'AG-DKR-001', name: 'Boutique Teranga', address: 'Marché Sandaga, Av. Émile Badiane',
    city: 'Dakar', country: 'Senegal', lat: 14.6737, lon: -17.4381,
    accepts: ['uusdc', 'ueurc'], feeBps: 100, maxPayout: 500000, settlements: 1284,
    payouts: [{ fiat: 'XOF', method: 'cash', feeBps: 0 }, { fiat: 'XOF', method: 'momo', feeBps: 40 }],
    hours: '08:00–20:00', phone: '+221 77 000 0001',
  },
  {
    id: 'AG-DKR-002', name: 'Pharmacie du Plateau', address: '12 Rue Carnot, Plateau',
    city: 'Dakar', country: 'Senegal', lat: 14.6690, lon: -17.4370,
    accepts: ['uusdc', 'ueurc', 'uyml'], feeBps: 120, maxPayout: 300000, settlements: 412,
    payouts: [{ fiat: 'XOF', method: 'cash', feeBps: 0 }, { fiat: 'XOF', method: 'bank', feeBps: 60 }],
    hours: '09:00–19:00', phone: '+221 77 000 0002',
  },
  {
    id: 'AG-ABJ-001', name: 'Alimentation Nouvelle', address: 'Rue des Jardins, Cocody',
    city: 'Abidjan', country: "Côte d'Ivoire", lat: 5.3600, lon: -3.9900,
    accepts: ['uusdc', 'ueurc'], feeBps: 90, maxPayout: 750000, settlements: 2301,
    payouts: [{ fiat: 'XOF', method: 'cash', feeBps: 0 }, { fiat: 'XOF', method: 'momo', feeBps: 35 }],
    hours: '07:30–21:00', phone: '+225 07 000 0003',
  },
  {
    id: 'AG-LOS-001', name: 'Adeola Superstore', address: '14 Adeola Odeku St, Victoria Island',
    city: 'Lagos', country: 'Nigeria', lat: 6.4281, lon: 3.4219,
    accepts: ['uusdc', 'ueurc', 'uyml'], feeBps: 110, maxPayout: 2000000, settlements: 3980,
    payouts: [{ fiat: 'NGN', method: 'cash', feeBps: 0 }, { fiat: 'NGN', method: 'bank', feeBps: 50 }, { fiat: 'NGN', method: 'card', feeBps: 150 }],
    hours: '08:00–22:00', phone: '+234 802 000 0004',
  },
  {
    id: 'AG-LOS-002', name: 'Ikeja Mobile Money', address: 'Computer Village, Ikeja',
    city: 'Lagos', country: 'Nigeria', lat: 6.6018, lon: 3.3515,
    accepts: ['uusdc'], feeBps: 130, maxPayout: 1000000, settlements: 1567,
    payouts: [{ fiat: 'NGN', method: 'cash', feeBps: 0 }, { fiat: 'NGN', method: 'momo', feeBps: 45 }],
    hours: '09:00–18:00', phone: '+234 802 000 0005',
  },
  {
    id: 'AG-NBO-001', name: 'Westlands Duka', address: 'Woodvale Grove, Westlands',
    city: 'Nairobi', country: 'Kenya', lat: -1.2670, lon: 36.8060,
    accepts: ['uusdc', 'ueurc'], feeBps: 100, maxPayout: 200000, settlements: 2745,
    payouts: [{ fiat: 'KES', method: 'cash', feeBps: 0 }, { fiat: 'KES', method: 'momo', feeBps: 30 }],
    hours: '07:00–21:00', phone: '+254 700 000 006',
  },
  {
    id: 'AG-NBO-002', name: 'Kibera Cash Point', address: 'Olympic Estate, Kibera',
    city: 'Nairobi', country: 'Kenya', lat: -1.3130, lon: 36.7830,
    accepts: ['uusdc'], feeBps: 80, maxPayout: 100000, settlements: 5120,
    payouts: [{ fiat: 'KES', method: 'momo', feeBps: 25 }],
    hours: '06:00–20:00', phone: '+254 700 000 007',
  },
  {
    id: 'AG-ACC-001', name: 'Osu Provisions', address: 'Oxford St, Osu',
    city: 'Accra', country: 'Ghana', lat: 5.5560, lon: -0.1820,
    accepts: ['uusdc', 'ueurc'], feeBps: 95, maxPayout: 50000, settlements: 1893,
    payouts: [{ fiat: 'GHS', method: 'cash', feeBps: 0 }, { fiat: 'GHS', method: 'momo', feeBps: 40 }],
    hours: '08:00–20:00', phone: '+233 24 000 0008',
  },
  {
    id: 'AG-JNB-001', name: 'Maboneng Exchange', address: 'Fox St, Maboneng',
    city: 'Johannesburg', country: 'South Africa', lat: -26.2050, lon: 28.0600,
    accepts: ['uusdc', 'ueurc', 'uyml'], feeBps: 85, maxPayout: 40000, settlements: 3211,
    payouts: [{ fiat: 'ZAR', method: 'cash', feeBps: 0 }, { fiat: 'ZAR', method: 'bank', feeBps: 45 }, { fiat: 'ZAR', method: 'card', feeBps: 130 }],
    hours: '08:30–18:00', phone: '+27 82 000 0009',
  },
  {
    id: 'AG-CAI-001', name: 'Zamalek Money Point', address: '26th July Corridor, Zamalek',
    city: 'Cairo', country: 'Egypt', lat: 30.0600, lon: 31.2200,
    accepts: ['ueurc', 'uusdc'], feeBps: 105, maxPayout: 100000, settlements: 2088,
    payouts: [{ fiat: 'EGP', method: 'cash', feeBps: 0 }, { fiat: 'EGP', method: 'card', feeBps: 140 }],
    hours: '09:00–22:00', phone: '+20 100 000 0010',
  },
  {
    id: 'AG-CAS-001', name: 'Épicerie Anfa', address: 'Bd d\'Anfa, Maârif',
    city: 'Casablanca', country: 'Morocco', lat: 33.5890, lon: -7.6320,
    accepts: ['ueurc', 'uusdc'], feeBps: 90, maxPayout: 30000, settlements: 1420,
    payouts: [{ fiat: 'MAD', method: 'cash', feeBps: 0 }, { fiat: 'MAD', method: 'bank', feeBps: 55 }],
    hours: '08:00–21:00', phone: '+212 6 00 000 011',
  },
  {
    id: 'AG-KLA-001', name: 'Nakasero Traders', address: 'Nakasero Market, Kampala',
    city: 'Kampala', country: 'Uganda', lat: 0.3170, lon: 32.5800,
    accepts: ['uusdc'], feeBps: 115, maxPayout: 5000000, settlements: 967,
    payouts: [{ fiat: 'UGX', method: 'cash', feeBps: 0 }, { fiat: 'UGX', method: 'momo', feeBps: 35 }],
    hours: '07:00–19:00', phone: '+256 77 000 0012',
  },
];

/**
 * Distance in kilometres between two points, by the haversine formula.
 *
 * Straight-line, not driving distance. That is the honest thing to show: this
 * app cannot know the roads, and a "2 km" that turns out to be across a river
 * is worse than a number the reader knows to be a rough guide.
 */
export function distanceKm(aLat: number, aLon: number, bLat: number, bLon: number): number {
  const R = 6371;
  const toRad = (d: number) => (d * Math.PI) / 180;
  const dLat = toRad(bLat - aLat);
  const dLon = toRad(bLon - aLon);
  const lat1 = toRad(aLat);
  const lat2 = toRad(bLat);
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(lat1) * Math.cos(lat2) * Math.sin(dLon / 2) ** 2;
  return 2 * R * Math.asin(Math.sqrt(h));
}

export interface NearbyAgent extends Agent {
  km: number;
}

/**
 * Agents near a point, nearest first.
 *
 * `cashIn` filters to agents that actually hand out that currency — an agent
 * who cannot give you naira is not a result, however close they are.
 */
/**
 * Shops near a point, optionally only those that can do the job.
 *
 * Filtered on both halves rather than one: a shop that takes the beneficiary's
 * currency but cannot produce the fiat they need is not a shop they can use, and
 * listing it wastes a journey. `accepts` is the crypto going in, `fiat` the
 * money coming out.
 */
export function nearby(
  lat: number,
  lon: number,
  accepts?: string,
  limit = 8,
  fiat?: string,
): NearbyAgent[] {
  return AGENTS
    .filter((a) => !accepts || a.accepts.includes(accepts))
    .filter((a) => !fiat || a.payouts.some((p) => p.fiat === fiat))
    .map((a) => ({ ...a, km: distanceKm(lat, lon, a.lat, a.lon) }))
    .sort((x, y) => x.km - y.km)
    .slice(0, limit);
}

/**
 * Ask the browser where we are.
 *
 * Resolves to null rather than throwing when permission is refused, because
 * refusing location is a legitimate choice and the screen must still work —
 * it falls back to letting somebody pick a city.
 */
export function locate(): Promise<{ lat: number; lon: number } | null> {
  return new Promise((resolve) => {
    if (!navigator.geolocation) return resolve(null);
    navigator.geolocation.getCurrentPosition(
      (pos) => resolve({ lat: pos.coords.latitude, lon: pos.coords.longitude }),
      () => resolve(null),
      { timeout: 8000, maximumAge: 300000 },
    );
  });
}

/** Cities to fall back to when location is unavailable or refused. */
export const CITIES = [
  { name: 'Dakar', lat: 14.6937, lon: -17.4441 },
  { name: 'Abidjan', lat: 5.3600, lon: -4.0083 },
  { name: 'Lagos', lat: 6.5244, lon: 3.3792 },
  { name: 'Nairobi', lat: -1.2864, lon: 36.8172 },
  { name: 'Accra', lat: 5.6037, lon: -0.1870 },
  { name: 'Johannesburg', lat: -26.2041, lon: 28.0473 },
  { name: 'Cairo', lat: 30.0444, lon: 31.2357 },
  { name: 'Casablanca', lat: 33.5731, lon: -7.5898 },
  { name: 'Kampala', lat: 0.3476, lon: 32.5825 },
];
