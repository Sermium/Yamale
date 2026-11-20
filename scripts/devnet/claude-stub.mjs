#!/usr/bin/env node
// A stand-in for the Messages API on the local devnet. Every draft is the
// same: 18kt Yellow Gold, no weight, one kind of stone with an unknown carat,
// so a person has to finish it, as with a real draft of most descriptions.
import { createServer } from 'node:http';

createServer((req, res) => {
  let body = '';
  req.on('data', (d) => { body += d; }).on('end', () => {
    if (req.method !== 'POST' || !req.url.startsWith('/v1/messages')) { res.statusCode = 404; res.end(); return; }
    const draft = { metal: '18kt Yellow Gold', weightGrams: '', stones: [{ kind: 'Emerald', carat: '', count: 0 }] };
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({
      id: 'msg_stub', type: 'message', role: 'assistant', model: JSON.parse(body).model,
      content: [{ type: 'text', text: JSON.stringify(draft) }], stop_reason: 'end_turn', stop_sequence: null,
      usage: { input_tokens: 300, output_tokens: 60 },
    }));
  });
}).listen(8098, '127.0.0.1', () => console.log('claude stub on http://127.0.0.1:8098'));
