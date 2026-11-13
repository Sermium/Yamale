/**
 * Limits the API and the chain enforce, mirrored so a form cannot submit what
 * will be refused. Lengths are UTF-8 bytes, as the Go side counts them.
 */

const bytes = (s: string) => new TextEncoder().encode(s).length;

/** The service wording of a benefit: at most 80 bytes once trimmed, and not empty. */
export function kindProblem(kind: string): 'empty' | 'long' | null {
  const k = kind.trim();
  if (k === '') return 'empty';
  return bytes(k) > 80 ? 'long' : null;
}

export interface ApplicationFields { name: string; contact: string; email: string; phone: string; country: string; message: string }

/** The first field loyalty-api would refuse, or null. Mirrors tools/loyaltyapi/applications.go. */
export function applicationProblem(f: ApplicationFields): keyof ApplicationFields | null {
  if (f.name.trim().length < 2 || bytes(f.name) > 120) return 'name';
  if (f.contact.trim().length < 2 || bytes(f.contact) > 120) return 'contact';
  if (!f.email.includes('@') || bytes(f.email) > 254) return 'email';
  if (bytes(f.phone) > 40) return 'phone';
  if (!/^[A-Z]{2}$/.test(f.country.trim().toUpperCase())) return 'country';
  if (bytes(f.message) > 2000) return 'message';
  return null;
}

/** What Save does while the stored look is read. */
export type LookLoad = 'loading' | 'none' | 'loaded' | 'failed';
export function canSaveLook(load: LookLoad, busy: boolean): boolean {
  return (load === 'none' || load === 'loaded') && !busy;
}
/** A finished read may fill the form only if nobody has touched it meanwhile. */
export function mayApplyLoad(edited: boolean): boolean { return !edited; }
