// Every request this page makes.
//
// One place, so the claim "no request body from this page contains a phrase or a
// private key" is a claim about a file somebody can read in a minute rather than
// about a codebase. src/storage.test.ts asserts it over the bodies this module
// actually builds, and the Go side refuses any field it does not know, so a body
// with a phrase in it is a 400 rather than something the coordinator ignores.
//
// credentials: 'omit' on every call. The ceremony sets no cookie and reads none;
// omitting them means a browser that had one for the host's domain — the page may
// be served under a path on a domain that runs other things — does not attach it
// to a ceremony request.

export type ApiError = Error & { status?: number };

export type Credential = { kind: 'coordinator' | 'custodian'; token: string };

async function call<T>(credential: Credential, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'X-Ceremony-Token': credential.token };
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  const response = await fetch(path, {
    method: body === undefined ? 'GET' : 'POST',
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: 'no-store',
    credentials: 'omit',
    redirect: 'error',
  });

  const text = await response.text();
  let parsed: unknown = null;
  try {
    parsed = text ? JSON.parse(text) : null;
  } catch {
    parsed = { error: text };
  }
  if (!response.ok) {
    // The server's sentence, verbatim. These messages are the ceremony's voice
    // at the moment something is wrong, and a page that replaced them with "an
    // error occurred" would be hiding the sentence somebody needs to act on.
    const message =
      parsed && typeof parsed === 'object' && 'error' in parsed
        ? String((parsed as { error: unknown }).error)
        : `the coordinator answered ${response.status}`;
    const error: ApiError = new Error(message);
    error.status = response.status;
    throw error;
  }
  return parsed as T;
}

export const api = {
  bundle: (c: Credential) => call<{ hash: string; files: Record<string, string> }>(c, 'api/bundle'),

  coordinatorState: (c: Credential) => call<unknown>(c, 'api/coordinator/state'),
  setup: (c: Credential, body: unknown) => call<unknown>(c, 'api/coordinator/setup', body),
  reissue: (c: Credential, name: string, reason: string) =>
    call<unknown>(c, 'api/coordinator/reissue', { name, reason }),
  exportRecord: (c: Credential, body: unknown) =>
    call<{ record: string; files: Record<string, string> }>(c, 'api/coordinator/export', body),

  invite: (c: Credential) => call<unknown>(c, 'api/invite'),
  opened: (c: Credential) => call<unknown>(c, 'api/invite/opened', {}),
  // The generation grant. Spent before the words are shown, not after: a page
  // that recorded the grant afterwards would let a reload between the two hand
  // out a second phrase, and then two sheets would exist for one custodian with
  // nothing to say which is the live one.
  generated: (c: Credential) => call<unknown>(c, 'api/invite/generated', {}),
  submit: (c: Credential, submission: unknown) => call<unknown>(c, 'api/invite/submission', submission),
  attest: (c: Credential, signed: unknown) => call<unknown>(c, 'api/invite/attestation', signed),
};
