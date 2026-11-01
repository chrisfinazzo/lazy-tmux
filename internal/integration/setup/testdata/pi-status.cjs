const assert = require('node:assert/strict');
const cp = require('node:child_process');
const path = require('node:path');
const { createJiti } = require(require.resolve('jiti', { paths: [process.argv[2]] }));

let bindings = new Map();
let unavailable = false;
cp.execFileSync = (command, args) => {
  if (unavailable) throw new Error('unavailable');
  if (command === 'ps') return 'Sun Nov 1 11:00:00 2026\n';
  assert.equal(command, 'tmux');
  if (args[0] === 'show-option') return bindings.get(args.at(-1)) ?? '';
  if (args[1] === '-pu') bindings.delete(args.at(-1));
  else bindings.set(args.at(-2), args.at(-1));
  return '';
};
const jiti = createJiti(__filename, { moduleCache: false, interopDefault: true });
const loaded = jiti(path.join(__dirname, '..', 'pi-extension.ts'));
const extension = loaded.default ?? loaded;
process.env.TMUX = '/tmp/test-socket,1,0';
process.env.TMUX_PANE = '%1';

function client() {
  bindings = new Map();
  const handlers = new Map();
  let idle = true;
  let file = '/tmp/session-a.jsonl';
  let id = 'session-a';
  const ctx = {
    hasUI: true, mode: 'tui',
    isIdle: () => idle,
    sessionManager: { getSessionFile: () => file, getSessionId: () => id },
  };
  extension({ on: (event, handler) => {
    const list = handlers.get(event) ?? [];
    list.push(handler); handlers.set(event, list);
    return () => {};
  } });
  return {
    ctx,
    emit(event, data = {}) { for (const handler of handlers.get(event) ?? []) handler(data, ctx); },
    state() { return JSON.parse(bindings.get('@lazy_tmux_pi_binding')).state; },
    binding() { return JSON.parse(bindings.get('@lazy_tmux_pi_binding')); },
    idle(value) { idle = value; },
    session(newID, newFile) { id = newID; file = newFile; },
  };
}

let c = client();
c.emit('session_start'); assert.equal(c.state(), 'idle');
assert.equal(c.binding().session_id, 'session-a');
c.idle(false); c.emit('before_agent_start'); assert.equal(c.state(), 'working');
c.emit('agent_start'); assert.equal(c.state(), 'working');
c.emit('ui_prompt_start', {kind:'confirm'}); assert.equal(c.state(), 'awaiting_decision');
c.emit('ui_prompt_start', {kind:'input'}); assert.equal(c.state(), 'awaiting_input');
c.emit('ui_prompt_end', {kind:'input'}); assert.equal(c.state(), 'awaiting_decision');
c.emit('ui_prompt_end', {kind:'confirm'}); assert.equal(c.state(), 'working');
c.emit('agent_end', {messages:[{role:'assistant',stopReason:'stop'}]}); assert.equal(c.state(), 'awaiting_input');
c.idle(true); c.emit('agent_settled', {aborted:false}); assert.equal(c.state(), 'awaiting_input');
c.emit('agent_start'); c.emit('agent_end', {messages:[{role:'assistant',stopReason:'error'}]});
assert.equal(c.state(), 'error');
c.emit('agent_start'); assert.equal(c.state(), 'working');
c.emit('agent_end', {messages:[{role:'assistant',stopReason:'aborted'}]});
c.emit('agent_settled', {aborted:true}); assert.equal(c.state(), 'awaiting_input');
c.emit('session_before_compact'); assert.equal(c.state(), 'working');
c.emit('session_compact'); assert.equal(c.state(), 'awaiting_input');
c.emit('session_before_compact'); c.emit('session_compact_failed', {aborted:false}); assert.equal(c.state(), 'error');
c.emit('session_before_compact'); c.emit('session_compact_failed', {aborted:true}); assert.equal(c.state(), 'awaiting_input');
const abortedCompaction = new AbortController();
c.emit('session_before_compact', {signal:abortedCompaction.signal}); assert.equal(c.state(), 'working');
abortedCompaction.abort(); assert.equal(c.state(), 'awaiting_input');

c.emit('agent_start'); c.emit('ui_prompt_start', {kind:'confirm'});
c.session('session-b','/tmp/session-b.jsonl'); c.emit('session_switch');
assert.equal(c.binding().session_id,'session-b'); assert.equal(c.state(),'idle');
c.session('session-c','/tmp/session-c.jsonl'); c.emit('session_fork');
assert.equal(c.binding().session_id,'session-c'); assert.equal(c.state(),'idle');
c.emit('session_shutdown'); assert.equal(bindings.has('@lazy_tmux_pi_binding'),false);

c = client(); c.session('transient', undefined); c.emit('session_start');
assert.equal(c.binding().file,''); assert.equal(c.binding().state,''); assert.equal(c.binding().session_id,'');
c = client(); c.ctx.mode='rpc'; c.emit('session_start'); assert.equal(c.binding().file,'');
c = client(); c.ctx.mode=undefined; c.emit('session_start'); assert.equal(c.state(),'idle');
c = client(); c.emit('session_start');
bindings.set('@lazy_tmux_pi_binding',JSON.stringify({pid:process.pid+1}));
c.emit('session_shutdown'); assert.equal(JSON.parse(bindings.get('@lazy_tmux_pi_binding')).pid,process.pid+1);
unavailable=true;
assert.doesNotThrow(()=>client());
assert.doesNotThrow(()=>c.emit('agent_start'));
assert.doesNotThrow(()=>c.emit('session_shutdown'));
console.log('Pi lifecycle status tests passed');
