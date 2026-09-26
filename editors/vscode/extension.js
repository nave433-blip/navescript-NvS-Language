// Minimal VS Code extension host for NvS.
//
// - Spawns `nvs lsp` (stdio) and speaks just enough LSP over
//   Content-Length framing to drive hover, go-to-definition,
//   completion, document/workspace symbols, references and
//   diagnostics. No npm dependencies: only the `vscode` API
//   (provided by the extension host) and Node's child_process.
// - Registers a debug adapter factory that spawns `nvs dap`
//   (Debug Adapter Protocol over stdio).
//
// Honest scope: this is a hand-written minimal client, not the
// full vscode-languageclient stack. It covers the requests the
// NvS server implements; anything else degrades silently.

const vscode = require('vscode');
const { spawn } = require('child_process');

let client = null;
let diagCollection = null;

function nvsBin() {
  return vscode.workspace.getConfiguration('nvs').get('path', 'nvs');
}

// ---------------------------------------------------------------------------
// Minimal LSP-over-stdio client.
// ---------------------------------------------------------------------------

function startServer() {
  const proc = spawn(nvsBin(), ['lsp'], { stdio: ['pipe', 'pipe', 'pipe'] });
  proc.stderr.on('data', d => console.error('[nvs lsp]', d.toString()));
  proc.on('exit', code => console.warn('[nvs lsp] exited with code', code));

  const pending = new Map();
  let seq = 0;
  let buf = Buffer.alloc(0);

  function handleMessage(msg) {
    if (msg.id !== undefined && pending.has(msg.id)) {
      const { resolve, reject } = pending.get(msg.id);
      pending.delete(msg.id);
      if (msg.error) reject(new Error(msg.error.message || 'LSP error'));
      else resolve(msg.result);
      return;
    }
    if (msg.method === 'textDocument/publishDiagnostics') {
      const uri = vscode.Uri.parse(msg.params.uri);
      const diags = (msg.params.diagnostics || []).map(d => new vscode.Diagnostic(
        new vscode.Range(d.range.start.line, d.range.start.character,
                         d.range.end.line, d.range.end.character),
        d.message,
        d.severity === 1 ? vscode.DiagnosticSeverity.Error
                         : vscode.DiagnosticSeverity.Warning
      ));
      if (diagCollection) diagCollection.set(uri, diags);
    }
  }

  proc.stdout.on('data', chunk => {
    buf = Buffer.concat([buf, chunk]);
    for (;;) {
      const idx = buf.indexOf('\r\n\r\n');
      if (idx < 0) return;
      const m = /Content-Length:\s*(\d+)/i.exec(buf.slice(0, idx).toString());
      if (!m) return;
      const len = parseInt(m[1], 10);
      if (buf.length < idx + 4 + len) return;
      try {
        handleMessage(JSON.parse(buf.slice(idx + 4, idx + 4 + len).toString()));
      } catch (e) { console.error('[nvs lsp] bad frame', e); }
      buf = buf.slice(idx + 4 + len);
    }
  });

  function send(msg) {
    const body = Buffer.from(JSON.stringify(msg));
    proc.stdin.write(Buffer.concat([
      Buffer.from(`Content-Length: ${body.length}\r\n\r\n`), body]));
  }
  function request(method, params) {
    const id = ++seq;
    return new Promise((resolve, reject) => {
      pending.set(id, { resolve, reject });
      setTimeout(() => {
        if (pending.delete(id)) reject(new Error('nvs lsp request timed out: ' + method));
      }, 8000);
      send({ jsonrpc: '2.0', id, method, params });
    });
  }
  function notify(method, params) { send({ jsonrpc: '2.0', method, params }); }

  return {
    request, notify,
    stop() {
      pending.forEach(({ reject }) => reject(new Error('client stopping')));
      pending.clear();
      try { proc.kill(); } catch (e) { /* already gone */ }
    }
  };
}

function docParams(doc, pos) {
  const p = { textDocument: { uri: doc.uri.toString() } };
  if (pos) p.position = { line: pos.line, character: pos.character };
  return p;
}

function toRange(r) {
  return new vscode.Range(r.start.line, r.start.character, r.end.line, r.end.character);
}

function toLocation(l) {
  return new vscode.Location(vscode.Uri.parse(l.uri), toRange(l.range));
}

function completionKind(n) {
  const m = vscode.CompletionItemKind;
  switch (n) {
    case 3: return m.Function;   // server: function
    case 6: return m.Variable;   // server: variable
    case 7: return m.Class;
    case 5: return m.Field;
    default: return m.Text;
  }
}

function symbolKind(n) {
  const m = vscode.SymbolKind;
  switch (n) {
    case 12: return m.Function;
    case 5: return m.Class;
    case 10: return m.Enum;
    case 11: return m.Interface;
    default: return m.Variable;
  }
}

// ---------------------------------------------------------------------------
// Activation.
// ---------------------------------------------------------------------------

function activate(context) {
  diagCollection = vscode.languages.createDiagnosticCollection('nvs');
  context.subscriptions.push(diagCollection);

  client = startServer();
  context.subscriptions.push({ dispose: () => { if (client) client.stop(); } });

  const folders = vscode.workspace.workspaceFolders;
  client.request('initialize', {
    processId: process.pid,
    rootUri: folders && folders[0] ? folders[0].uri.toString() : null,
    capabilities: {}
  }).catch(e => console.error('[nvs lsp] initialize failed:', e.message));
  client.notify('initialized', {});

  const syncOpen = doc => {
    if (doc.languageId !== 'nvs') return;
    client.notify('textDocument/didOpen', {
      textDocument: {
        uri: doc.uri.toString(), languageId: 'nvs',
        version: doc.version, text: doc.getText()
      }
    });
  };
  vscode.workspace.textDocuments.forEach(syncOpen);
  context.subscriptions.push(vscode.workspace.onDidOpenTextDocument(syncOpen));
  context.subscriptions.push(vscode.workspace.onDidChangeTextDocument(e => {
    if (e.document.languageId !== 'nvs') return;
    client.notify('textDocument/didChange', {
      textDocument: { uri: e.document.uri.toString(), version: e.document.version },
      contentChanges: [{ text: e.document.getText() }]
    });
  }));
  context.subscriptions.push(vscode.workspace.onDidCloseTextDocument(doc => {
    client.notify('textDocument/didClose', {
      textDocument: { uri: doc.uri.toString() }
    });
  }));

  const sel = { language: 'nvs' };
  const safe = fn => async (...args) => {
    try { return await fn(...args); }
    catch (e) { return null; } // server doesn't implement it / timed out
  };

  context.subscriptions.push(vscode.languages.registerHoverProvider(sel, {
    provideHover: safe(async (doc, pos) => {
      const r = await client.request('textDocument/hover', docParams(doc, pos));
      if (r && r.contents && r.contents.value)
        return new vscode.Hover(new vscode.MarkdownString(r.contents.value));
      return null;
    })
  }));

  context.subscriptions.push(vscode.languages.registerDefinitionProvider(sel, {
    provideDefinition: safe(async (doc, pos) => {
      const r = await client.request('textDocument/definition', docParams(doc, pos));
      return (r || []).map(toLocation);
    })
  }));

  context.subscriptions.push(vscode.languages.registerReferenceProvider(sel, {
    provideReferences: safe(async (doc, pos) => {
      const r = await client.request('textDocument/references',
        { ...docParams(doc, pos), context: { includeDeclaration: true } });
      return (r || []).map(toLocation);
    })
  }));

  context.subscriptions.push(vscode.languages.registerCompletionItemProvider(sel, {
    provideCompletionItems: safe(async (doc, pos) => {
      const r = await client.request('textDocument/completion', docParams(doc, pos));
      const items = (r && r.items) || [];
      return items.map(it => {
        const c = new vscode.CompletionItem(it.label, completionKind(it.kind));
        if (it.detail) c.detail = it.detail;
        return c;
      });
    })
  }, '.', '('));

  const toSymbol = s => new vscode.DocumentSymbol(
    s.name, '', symbolKind(s.kind), toRange(s.range), toRange(s.selectionRange));

  context.subscriptions.push(vscode.languages.registerDocumentSymbolProvider(sel, {
    provideDocumentSymbols: safe(async doc => {
      const r = await client.request('textDocument/documentSymbol', docParams(doc));
      return (r || []).map(toSymbol);
    })
  }));

  context.subscriptions.push(vscode.languages.registerWorkspaceSymbolProvider({
    provideWorkspaceSymbols: safe(async query => {
      const r = await client.request('workspace/symbol', { query: query || '' });
      return (r || []).map(s => new vscode.SymbolInformation(
        s.name, symbolKind(s.kind), '', toLocation(s.location)));
    })
  }));

  // Debug adapter: `nvs dap` speaks DAP over stdio, which is exactly
  // what DebugAdapterExecutable wires up.
  context.subscriptions.push(
    vscode.debug.registerDebugAdapterDescriptorFactory('nvs', {
      createDebugAdapterDescriptor() {
        return new vscode.DebugAdapterExecutable(nvsBin(), ['dap']);
      }
    }));
}

function deactivate() {
  if (client) { client.stop(); client = null; }
}

module.exports = { activate, deactivate };
