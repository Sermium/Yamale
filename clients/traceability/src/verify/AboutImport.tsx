import { t } from '@yamale/chain';

import { AppBar } from '../ui.tsx';

/** https://traceability.yamaletech.com/about-import, named in every request the catalogue reader makes. */
export function AboutImport() {
  return (
    <>
      <AppBar title={t('traceability.aboutImportTitle')} eyebrow={t('traceability.title')} />
      <main className="y-screen prose">
        <p>{t('traceability.aboutImport1')}</p>
        <p>{t('traceability.aboutImport2')}</p>
        <p>{t('traceability.aboutImport3')}</p>
      </main>
    </>
  );
}
