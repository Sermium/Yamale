import { t, translateError } from '@yamale/chain';

/**
 * A question that could not be asked, said as such.
 *
 * This component exists because of one substitution that appeared six times in
 * this application: `query.data ?? []`. It renders a node that did not answer
 * exactly as it renders an answer of nothing — and on a treasury console those
 * are opposite facts. "No commitments: every coin this treasury holds is
 * available to spend" printed on the strength of a failed request is not a
 * blank screen, it is a false statement about somebody's money, in the place
 * they went to check.
 *
 * So every read on these pages now has three renderings — waiting, unknown,
 * and the answer — and this is the middle one. The raw fault stays behind a
 * disclosure: a treasurer does not need it, and the person they telephone
 * cannot work without it.
 */
export function Unknown({
  what,
  error,
  onRetry,
}: {
  /** What is unknown, as a statement. Not "error" — that is not the news. */
  what: string;
  error: unknown;
  onRetry?: () => void;
}) {
  const translated = translateError(error instanceof Error ? error.message : String(error));

  return (
    <div className="notice">
      <strong>{what}</strong> {translated.message}.{translated.reason ? ` ${translated.reason}` : ''}
      {onRetry && (
        <p className="unknown__act">
          <button type="button" className="chip" onClick={onRetry}>
            {t('safe.tryAgain')}
          </button>
        </p>
      )}
      <details className="payload">
        <summary>{t('safe.whatTheNodeSaid')}</summary>
        <pre className="payload__pre">{translated.raw}</pre>
      </details>
    </div>
  );
}
