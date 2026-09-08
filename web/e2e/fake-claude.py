#!/usr/bin/env python3
"""Deterministic local harness for the isolated Playwright workflow test."""
import json, re, sys, urllib.request, uuid, os, time
from pathlib import Path
codex = 'exec' in sys.argv
if '--mcp-config' not in sys.argv and not codex:
    print('fake-claude 1.0 (isolated test)')
    sys.exit(0)
if codex:
    assert 'model_reasoning_effort="high"' in sys.argv, 'assigned worker effort was lost'
    assert 'assigned-model' in sys.argv, 'assigned worker model was lost'
    assert 'Follow the assigned worker instructions.' in ' '.join(sys.argv), 'assigned worker instructions were lost'
    config = dict(arg.split('=', 1) for arg in sys.argv if arg.startswith('mcp_servers.flywheel.'))
    server = {'url': json.loads(config['mcp_servers.flywheel.url']), 'headers': dict(re.findall(r'"([^"]+)"\s*=\s*"([^"]+)"', config['mcp_servers.flywheel.http_headers']))}
else:
    with open(sys.argv[sys.argv.index('--mcp-config') + 1]) as handle:
        server = json.load(handle)['mcpServers']['flywheel']
url = server['url']
if not url.startswith('http://127.0.0.1:8091/'):
    raise RuntimeError('Fake harness may only call the isolated test server')
# Legacy SSE configurations and Streamable HTTP share the same MCP backend.
url = 'http://127.0.0.1:8091/mcp'
headers = {**server['headers'], 'Content-Type': 'application/json', 'Accept': 'application/json, text/event-stream'}
def call(name, args):
    request = urllib.request.Request(url, json.dumps({'jsonrpc': '2.0', 'id': str(uuid.uuid4()), 'method': 'tools/call', 'params': {'name': name, 'arguments': args}}).encode(), headers)
    with urllib.request.urlopen(request, timeout=10) as response:
        raw = response.read().decode()
    if raw.startswith('event:') or raw.startswith('data:'):
        raw = next(line[5:].strip() for line in raw.splitlines() if line.startswith('data:'))
    result = json.loads(raw)
    if 'error' in result:
        raise RuntimeError(result['error'])
    result = result['result']
    if result.get('isError'):
        raise RuntimeError(result['content'])
    return json.loads(result['content'][0]['text'])
text = ' '.join(sys.argv)
ticket = re.search(r'Flywheel ticket ([\w-]+)', text).group(1)
project = re.search(r'project_id "([\w-]+)"', text).group(1)
claimed = call('claim_ticket', {'ticket_id': ticket, 'project_id': project})
assert claimed['ticket']['id'] == ticket
lease = claimed['lease']['token']
call('start_ticket', {'ticket_id': ticket, 'lease_token': lease})
session_id = str(uuid.uuid4())
print(json.dumps({'type': 'thread.started', 'thread_id': session_id} if codex else {'type': 'system', 'subtype': 'init', 'session_id': session_id}), flush=True)
if codex:
    print(json.dumps({'type': 'item.started', 'item': {'type': 'command_execution', 'command': 'test-only command'}}), flush=True)
    print(json.dumps({'type': 'item.completed', 'item': {'type': 'reasoning', 'text': 'This content should not be copied into the tray'}}), flush=True)
else:
    print(json.dumps({'type': 'assistant', 'message': {'id': 'm1', 'content': [{'type': 'tool_use', 'name': 'Read'}, {'type': 'thinking', 'thinking': 'This content should not be copied into the tray'}], 'usage': {'input_tokens': 25, 'output_tokens': 7}}}), flush=True)
hold_dir = os.getenv('FLYWHEEL_E2E_HARNESS_HOLD_DIR')
if hold_dir:
    root = Path(hold_dir)
    assert root.name == 'holds' and root.parent.name.startswith('flywheel-hardening.'), 'Expected disposable test directory'
    release = root / (ticket + ('-codex.release' if codex else '-claude.release'))
    deadline = time.monotonic() + 45
    while not release.exists() and time.monotonic() < deadline:
        time.sleep(0.1)
    assert release.exists(), 'Playwright did not release the test harness'

# This must fail for an implementation run, even though its parent is an operator key.
try:
    call('approve_ticket', {'ticket_id': ticket})
    raise AssertionError('executor approved its own work')
except RuntimeError:
    pass
call('submit_ticket', {'ticket_id': ticket, 'lease_token': lease, 'outputs': json.dumps({'summary': 'Completed by isolated fake harness'})})
if codex:
    for event in [{'type': 'item.completed', 'item': {'type': 'agent_message', 'text': 'Completed by isolated fake harness'}}, {'type': 'turn.completed'}]:
        print(json.dumps(event))
else:
    print(json.dumps({'type': 'result', 'subtype': 'success', 'is_error': False, 'session_id': session_id, 'num_turns': 1, 'result': 'Completed by isolated fake harness'}))
