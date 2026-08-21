// The smallest DOM layer that keeps two rules.
//
// Nothing is stored. No localStorage, no sessionStorage, no IndexedDB, no
// cookies, not even a remembered theme. The page tells the custodian to open the
// link in a private window, and a web page cannot make that happen — it is a
// browser boundary — so the honest response is to make private mode matter as
// little as possible by holding nothing worth finding. src/storage.test.ts reads
// the built bundle and fails if any storage API appears in it, including one
// pulled in by a dependency.
//
// Nothing is built from a string. Every value that reaches the document goes in
// as textContent, never as markup. Custodian names come from the coordinator and
// error messages come from the server, and a page that interpolated either into
// innerHTML would let whoever chose a name run script in four other custodians'
// browsers — on the one screen in this product where a custodian is looking at
// twenty-four words.

export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: Record<string, string> = {},
  children: Array<Node | string> = [],
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (key === 'class') node.className = value;
    else node.setAttribute(key, value);
  }
  for (const child of children) {
    node.append(typeof child === 'string' ? document.createTextNode(child) : child);
  }
  return node;
}

export function clear(node: Element): void {
  while (node.firstChild) node.removeChild(node.firstChild);
}

export function panel(title: string, children: Array<Node | string>): HTMLElement {
  return el('section', { class: 'panel' }, [el('h2', {}, [title]), ...children]);
}

export function paragraph(...parts: Array<Node | string>): HTMLElement {
  return el('p', {}, parts);
}

export function muted(text: string): HTMLElement {
  return el('p', { class: 'muted small' }, [text]);
}

export function mono(text: string): HTMLElement {
  return el('span', { class: 'mono' }, [text]);
}

export function button(label: string, onClick: () => void, kind = ''): HTMLButtonElement {
  const node = el('button', kind ? { class: kind } : {}, [label]);
  node.addEventListener('click', onClick);
  return node;
}

export function row(...children: Array<Node | string>): HTMLElement {
  return el('div', { class: 'row' }, children);
}

export function field(label: string, input: HTMLElement): HTMLElement {
  return el('div', {}, [el('label', {}, [label]), input]);
}

export function textInput(value = '', placeholder = ''): HTMLInputElement {
  const input = el('input', { type: 'text', placeholder, autocomplete: 'off', spellcheck: 'false' });
  input.value = value;
  return input;
}

// details/summary rather than a modal.
//
// The trust material used to be a screen somebody had to click past. For a
// rehearsal the coordinator is also every custodian, and a page that made them
// acknowledge that they might not trust themselves reads as noise — the kind of
// noise people learn to dismiss, which is how a real warning gets dismissed
// later. So it is one line with a disclosure: there for the custodian who wants
// it, never in the way of the ones who do not.
export function disclosure(summary: string, children: Array<Node | string>): HTMLElement {
  return el('details', { class: 'note' }, [el('summary', {}, [summary]), ...children]);
}

// showFingerprint renders a value that will be read aloud over a telephone.
//
// Grouped, spaced and in a font that keeps l/1/I and 0/O apart, because five
// custodians arguing about whether a character was a one or an l is the failure
// this styling exists to prevent. The groups are separate elements so the browser
// breaks between them rather than mid-group on a narrow screen.
export function showFingerprint(value: string): HTMLElement {
  const groups = value.split('-');
  const node = el('div', { class: 'fingerprint' });
  groups.forEach((group, index) => {
    node.append(el('span', { class: 'fingerprint__g' }, [group]));
    if (index < groups.length - 1) node.append(el('span', { class: 'fingerprint__d' }, ['-']));
  });
  return node;
}

export function errorBox(message: string): HTMLElement {
  return el('div', { class: 'err', role: 'alert' }, [message]);
}

export function statusPill(text: string, kind: string): HTMLElement {
  return el('span', { class: `pill pill--${kind}` }, [text]);
}
