import { t } from '@yamale/chain';

/** A failure in words, with whatever the service said kept under Advanced. */
export function FailureNote({ text, raw }: { text: string; raw: string }) {
  return (
    <div className="note note--warn" role="alert">
      <p>{text}</p>
      {raw && <details className="advanced"><summary>{t('traceability.advanced')}</summary><pre>{raw}</pre></details>}
    </div>
  );
}
