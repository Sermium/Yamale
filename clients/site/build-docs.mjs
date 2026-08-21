/**
 * Render docs/ into static HTML for the presentation site.
 *
 * The documentation is written in Markdown and read on GitHub, but the site
 * serves plain files — so a link to a guide returned the file, or a 404, rather
 * than a page. This turns the same sources into HTML at deploy time.
 *
 * Rendering rather than hand-writing matters for the same reason
 * docs/reference is generated: two copies of an explanation drift, and the one
 * on the website is the copy nobody remembers to update.
 *
 *   node clients/site/build-docs.mjs
 */
import { readdir, readFile, mkdir, writeFile, copyFile } from 'node:fs/promises';
import { join, relative, dirname, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import { marked } from 'marked';

const here = dirname(fileURLToPath(import.meta.url));
const repo = join(here, '..', '..');
const docsRoot = join(repo, 'docs');
const outRoot = join(here, 'docs');

/** Every .md under docs/, depth-first, as repo-relative paths. */
async function walk(dir) {
  const found = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) found.push(...(await walk(full)));
    else if (entry.name.endsWith('.md')) found.push(full);
  }
  return found;
}

/** Titles come from the first `# ` heading, falling back to the filename. */
function titleOf(markdown, file) {
  const match = markdown.match(/^#\s+(.+)$/m);
  return match ? match[1].trim() : basename(file, '.md');
}

const page = (title, body, depth, cls = 'doc') => {
  const up = '../'.repeat(depth);
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${title} — Yamale</title>
<link rel="icon" type="image/svg+xml" href="${up}../mark.svg">
<link rel="stylesheet" href="${up}doc.css">
</head>
<body>
<header class="docbar">
  <a class="docbar__brand" href="${up}index.html"><svg class="brand__mark" viewBox="0 0 64 64" aria-hidden="true"><rect x="4" y="4" width="56" height="56" rx="7" fill="#12253F"/><path d="M17 17 L32 32 L47 17" fill="none" stroke="#FFFFFF" stroke-width="7.2"/><path d="M32 32 L32 49.5" fill="none" stroke="#A87B3C" stroke-width="7.2"/></svg> Yamale <span>docs</span></a>
  <a href="${up}../index.html">← back to the site</a>
</header>
<main class="${cls}">
${body}
</main>
</body>
</html>
`;
};

await mkdir(outRoot, { recursive: true });

// The stylesheet lives *inside* docs/, because every page links to it relative
// to this directory. Leaving it at the site root made each page ask for
// /docs/doc.css and get a 404 — which renders as unstyled Markdown and looks
// exactly like the generator having failed.
await copyFile(join(here, 'doc.css'), join(outRoot, 'doc.css'));

const files = await walk(docsRoot);
const index = [];

for (const file of files) {
  const markdown = await readFile(file, 'utf8');
  const rel = relative(docsRoot, file);
  const outPath = join(outRoot, rel.replace(/\.md$/, '.html'));
  const depth = rel.split(/[\\/]/).length - 1;

  // Links between documents point at .md files, which do not exist here.
  const body = marked.parse(
    markdown.replace(/\]\(([^)]+?)\.md(#[^)]*)?\)/g, (_m, path, hash) => `](${path}.html${hash ?? ''})`),
  );

  await mkdir(dirname(outPath), { recursive: true });
  await writeFile(outPath, page(titleOf(markdown, file), body, depth));
  index.push({ href: rel.replace(/\.md$/, '.html').replace(/\\/g, '/'), title: titleOf(markdown, file) });
}

// The landing page, grouped by directory so guides and reference stay apart.
const groups = new Map();
for (const entry of index) {
  const group = entry.href.includes('/') ? entry.href.split('/')[0] : 'overview';
  if (!groups.has(group)) groups.set(group, []);
  groups.get(group).push(entry);
}

// A directory name is not a section heading, and a reader deciding where to
// click needs to know what a section is *for* before they know what is in it.
const SECTIONS = {
  overview: { name: 'Start here', blurb: 'What Yamale is, in one document.' },
  guides: { name: 'Guides', blurb: 'How to do a thing, end to end, against a real chain.' },
  reference: {
    name: 'Reference',
    blurb: 'Every message, query, parameter and error code — generated from the source, never hand-written.',
  },
  scope: { name: 'Scope', blurb: 'What is being built and why, including the decisions taken against.' },
};

const order = ['overview', 'guides', 'reference', 'scope'];
const rank = (g) => (order.indexOf(g) === -1 ? order.length : order.indexOf(g));

const sections = [...groups.entries()]
  .sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
  .map(([group, entries]) => {
    const meta = SECTIONS[group] ?? { name: group, blurb: '' };
    const links = entries
      .sort((a, b) => a.title.localeCompare(b.title))
      .map((e) => `      <li><a href="${e.href}">${e.title}</a></li>`)
      .join('\n');
    return `  <section class="card">
    <h2>${meta.name}</h2>
    ${meta.blurb ? `<p class="card__blurb">${meta.blurb}</p>` : ''}
    <ul>
${links}
    </ul>
  </section>`;
  })
  .join('\n');

await writeFile(
  join(outRoot, 'index.html'),
  page(
    'Documentation',
    `<h1>Yamale documentation</h1>
<p class="lede">Written as Markdown in the repository and rendered here, so the
website copy cannot drift from the one developers read.</p>
<div class="cards">
${sections}
</div>`,
    0,
    'doc doc--index',
  ),
);

console.log(`rendered ${files.length} documents into ${relative(repo, outRoot)}`);
